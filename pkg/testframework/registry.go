package testframework

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/distribution/configuration"
	"golang.org/x/crypto/bcrypt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	routev1 "github.com/openshift/api/route/v1"
	routeclient "github.com/openshift/client-go/route/clientset/versioned"

	"github.com/openshift/image-registry/pkg/cmd/dockerregistry"
	registryconfig "github.com/openshift/image-registry/pkg/dockerregistry/server/configuration"
	"github.com/openshift/image-registry/pkg/testutil"
)

type CloseFunc func() error

type RegistryOption interface {
	Apply(dockerConfig *configuration.Configuration, extraConfig *registryconfig.Configuration)
}

type DisableMirroring struct{}

func (o DisableMirroring) Apply(dockerConfig *configuration.Configuration, extraConfig *registryconfig.Configuration) {
	extraConfig.Pullthrough.Mirror = false
}

type EnableMetrics struct {
	Secret string
}

func (o EnableMetrics) Apply(dockerConfig *configuration.Configuration, extraConfig *registryconfig.Configuration) {
	extraConfig.Metrics.Enabled = true
	extraConfig.Metrics.Secret = o.Secret
}

func StartTestRegistry(t *testing.T, kubeConfigPath string, options ...RegistryOption) (net.Listener, CloseFunc) {
	localIPv4, err := DefaultLocalIP4()
	if err != nil {
		t.Fatalf("failed to detect an IPv4 address which would be reachable from containers: %v", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", localIPv4, 0))
	if err != nil {
		t.Fatalf("failed to listen on a port: %v", err)
	}

	dockerConfig := &configuration.Configuration{
		Version: "0.1",
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"delete": configuration.Parameters{
				"enabled": true,
			},
		},
		Auth: configuration.Auth{
			"openshift": configuration.Parameters{},
		},
		Middleware: map[string][]configuration.Middleware{
			"registry":   {{Name: "openshift"}},
			"repository": {{Name: "openshift"}},
			"storage":    {{Name: "openshift"}},
		},
	}
	dockerConfig.Log.Level = "debug"

	extraConfig := &registryconfig.Configuration{
		KubeConfig: kubeConfigPath,
		Server: &registryconfig.Server{
			Addr: ln.Addr().String(),
		},
		Pullthrough: &registryconfig.Pullthrough{
			Enabled: true,
			Mirror:  true,
		},
		Quota: &registryconfig.Quota{
			Enabled:  false,
			CacheTTL: 1 * time.Minute,
		},
		Cache: &registryconfig.Cache{
			BlobRepositoryTTL: 10 * time.Minute,
		},
		Compatibility: &registryconfig.Compatibility{
			AcceptSchema2: true,
		},
	}

	for _, opt := range options {
		opt.Apply(dockerConfig, extraConfig)
	}

	if err := registryconfig.InitExtraConfig(dockerConfig, extraConfig); err != nil {
		t.Fatalf("unable to init registry config: %v", err)
	}

	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)
	srv, err := dockerregistry.NewServer(ctx, dockerConfig, extraConfig)
	if err != nil {
		t.Fatalf("failed to create a new server: %v", err)
	}

	closed := int32(0)
	go func() {
		err := srv.Serve(ln)
		if atomic.LoadInt32(&closed) == 0 {
			// We cannot call t.Fatal here, because it's a different goroutine.
			panic(fmt.Errorf("failed to serve the image registry: %v", err))
		}
	}()
	close := func() error {
		atomic.StoreInt32(&closed, 1)
		return ln.Close()
	}

	return ln, close
}

type Registry struct {
	t        *testing.T
	listener net.Listener
	closeFn  CloseFunc
}

func (r *Registry) Close() {
	if err := r.closeFn(); err != nil {
		r.t.Fatalf("failed to close the registry's listener: %v", err)
	}
}

func (r *Registry) Addr() string {
	return r.listener.Addr().String()
}

func (r *Registry) BaseURL() string {
	return "http://" + r.listener.Addr().String()
}

func (r *Registry) Repository(namespace string, imagestream string, user *User) *Repository {
	creds := testutil.NewBasicCredentialStore(user.Name, user.Token)

	baseURL := r.BaseURL()
	repoName := fmt.Sprintf("%s/%s", namespace, imagestream)

	transport, err := testutil.NewTransport(baseURL, repoName, creds)
	if err != nil {
		r.t.Fatalf("failed to get transport for %s: %v", repoName, err)
	}

	repo, err := testutil.NewRepository(repoName, baseURL, transport)
	if err != nil {
		r.t.Fatalf("failed to get repository %s: %v", repoName, err)
	}

	return &Repository{
		Repository: repo,
		baseURL:    baseURL,
		repoName:   repoName,
		transport:  transport,
	}
}

type AuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type DockerConfig struct {
	Auths map[string]AuthConfig `json:"auths"`
}

func MakeDockerConfigSecret(name string, config *DockerConfig) (*corev1.Secret, error) {
	buf, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string][]byte{
			".dockerconfigjson": buf,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}, nil
}

type CleanupFunc func()

func CreateEphemeralRegistry(t *testing.T, restConfig *rest.Config, namespace string, accounts map[string]string) (string, string, CleanupFunc) {
	ctx := context.Background()

	kubeClient, err := kubeclient.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("failed to create Kubernetes client: %s", err)
	}

	routeClient, err := routeclient.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("failed to create OpenShift route client: %s", err)
	}

	name := "ephemeral-registry-" + utilrand.String(5)

	var cleaners []CleanupFunc
	cleanup := func() {
		for _, c := range cleaners {
			c()
		}
		t.Logf("deleted ephemeral registry %s", name)
	}

	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	var env []corev1.EnvVar
	if accounts != nil {
		var b bytes.Buffer
		for user, password := range accounts {
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				t.Fatalf("failed to create bcrypt hash of password: %s", err)
			}
			fmt.Fprintf(&b, "%s:%s\n", user, hash)
		}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Data: map[string][]byte{
				"htpasswd": b.Bytes(),
			},
		}

		_, err = kubeClient.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to create secret for ephemeral registry: %s", err)
		}
		cleaners = append(cleaners, func() {
			err = kubeClient.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
			if err != nil {
				t.Errorf("failed to delete secret %s: %s", name, err)
			}
		})

		volumes = append(volumes, corev1.Volume{
			Name: "auth",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: name,
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "auth",
			MountPath: "/auth",
		})
		env = append(
			env,
			corev1.EnvVar{
				Name:  "REGISTRY_AUTH",
				Value: "htpasswd",
			},
			corev1.EnvVar{
				Name:  "REGISTRY_AUTH_HTPASSWD_REALM",
				Value: "Registry",
			},
			corev1.EnvVar{
				Name:  "REGISTRY_AUTH_HTPASSWD_PATH",
				Value: "/auth/htpasswd",
			},
		)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"name": name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "registry",
					Image: "docker.io/library/registry:2.7.1",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 5000,
						},
					},
					Env: env,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("50Mi"),
						},
					},
					VolumeMounts: mounts,
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt(5000),
							},
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt(5000),
							},
						},
					},
					TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
				},
			},
			Volumes: volumes,
		},
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 5000,
				},
			},
			Selector: map[string]string{
				"name": name,
			},
		},
	}

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: name,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(5000),
			},
			TLS: &routev1.TLSConfig{
				Termination: routev1.TLSTerminationEdge,
			},
		},
	}

	_, err = kubeClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create ephemeral registry pod: %s", err)
	}
	cleaners = append(cleaners, func() {
		err = kubeClient.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Errorf("failed to delete pod %s: %s", name, err)
		}

		var lastErr error
		err = wait.Poll(time.Second, 30*time.Second, func() (done bool, err error) {
			pod, err := kubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				lastErr = err
				t.Logf("unable to check if pod %s is deleted: %s", name, err)
			}
			lastErr = fmt.Errorf("pod %s is alive (deletionTimestamp: %s)", name, pod.DeletionTimestamp)
			return false, nil
		})
		if err != nil {
			t.Errorf("failed to delete pod %s: %s", name, lastErr)
			return
		}
	})

	_, err = kubeClient.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service for ephemeral registry: %s", err)
	}
	cleaners = append(cleaners, func() {
		err = kubeClient.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Errorf("failed to delete service %s: %s", name, err)
		}
	})

	_, err = routeClient.RouteV1().Routes(namespace).Create(ctx, route, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create route for ephemeral registry: %s", err)
	}
	cleaners = append(cleaners, func() {
		err = routeClient.RouteV1().Routes(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			t.Errorf("failed to delete route %s: %s", name, err)
		}
	})

	var lastErr error
	err = wait.Poll(time.Second, 30*time.Second, func() (done bool, err error) {
		pod, err = kubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			return false, nil
		}
		if pod.Status.Phase != "Running" {
			lastErr = fmt.Errorf("pod phase is %s, want Running", pod.Status.Phase)
			return false, nil
		}
		for _, c := range pod.Status.ContainerStatuses {
			if !c.Ready {
				lastErr = fmt.Errorf("container %s is not ready (restartCount: %d, state: %v)", c.Name, c.RestartCount, c.State)
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("failed to wait until pod %s is ready: %v", name, lastErr)
	}

	err = wait.Poll(time.Second, 30*time.Second, func() (stop bool, err error) {
		_, err = kubeClient.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("failed to wait for %s endpoints: %v", name, lastErr)
	}

	var host string
	err = wait.Poll(time.Second, 30*time.Second, func() (done bool, err error) {
		route, err = routeClient.RouteV1().Routes(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			return false, nil
		}
		for _, ingress := range route.Status.Ingress {
			if len(ingress.Host) == 0 {
				continue
			}

			if err := checkRoute(ingress.Host); err != nil {
				lastErr = err
				return false, nil
			}

			host = ingress.Host
			return true, nil
		}
		lastErr = fmt.Errorf("route %s does not have ingress hosts", name)
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to wait until route %s is ready: %v", name, lastErr)
	}

	t.Logf("created ephemeral registry: %s (%s)", host, name)
	return host, name, cleanup
}

func checkRoute(host string) error {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	url := fmt.Sprintf("https://%s/v2/", host)
	client := &http.Client{Transport: tr}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// if deployed registry leverages basic authentication a StatusUnauthorized will
	// be returned so we consider this status as valid as well.
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusUnauthorized {
		dt, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("service not available: %s", string(dt))
	}
	return nil
}
