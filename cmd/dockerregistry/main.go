package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/klog/v2"

	"github.com/openshift/library-go/pkg/serviceability"

	"github.com/openshift/image-registry/pkg/cmd/dockerregistry"
	"github.com/openshift/image-registry/pkg/version"
)

var klogFlags *flag.FlagSet

func init() {
	klogFlags = flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)
}

func klogLogLevel() string {
	if klog.V(4).Enabled() {
		return "debug"
	}
	if klog.V(2).Enabled() {
		return "info"
	}
	if klog.V(1).Enabled() {
		return "warn"
	}
	return "error"
}

func main() {
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	// Sync the glog and klog flags.
	flag.CommandLine.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			_ = f2.Value.Set(value)
		}
	})

	flag.CommandLine.Visit(func(f1 *flag.Flag) {
		if f1.Name == "v" {
			if err := os.Setenv("REGISTRY_LOG_LEVEL", klogLogLevel()); err != nil {
				log.Fatalf("Unable to set REGISTRY_LOG_LEVEL: %v", err)
			}
		}
	})

	defer serviceability.BehaviorOnPanic(os.Getenv("OPENSHIFT_ON_PANIC"), version.Get())()
	defer serviceability.Profile(os.Getenv("OPENSHIFT_PROFILE")).Stop()
	startProfiler()

	rand.Seed(time.Now().UTC().UnixNano())
	runtime.GOMAXPROCS(runtime.NumCPU())

	// TODO convert to flags instead of a config file?
	configurationPath := ""
	if flag.NArg() > 0 {
		configurationPath = flag.Arg(0)
	}
	if configurationPath == "" {
		configurationPath = os.Getenv("REGISTRY_CONFIGURATION_PATH")
	}

	if configurationPath == "" {
		fmt.Println("configuration path unspecified")
		os.Exit(1)
	}
	// Prevent a warning about unrecognized environment variable
	if err := os.Unsetenv("REGISTRY_CONFIGURATION_PATH"); err != nil {
		log.Fatalf("Unable to unset REGISTRY_CONFIGURATION_PATH: %v", err)
	}

	configFile, err := os.Open(configurationPath)
	if err != nil {
		log.Fatalf("Unable to open configuration file: %s", err)
	}

	dockerregistry.Execute(configFile)
}

func env(key string, defaultValue string) string {
	val := os.Getenv(key)
	if len(val) == 0 {
		return defaultValue
	}
	return val
}

func startProfiler() {
	if env("OPENSHIFT_PROFILE", "") == "web" {
		go func() {
			runtime.SetBlockProfileRate(1)
			profilePort := env("OPENSHIFT_PROFILE_PORT", "6060")
			profileHost := env("OPENSHIFT_PROFILE_HOST", "127.0.0.1")
			log.Infof(fmt.Sprintf("Starting profiling endpoint at http://%s:%s/debug/pprof/", profileHost, profilePort))
			log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%s", profileHost, profilePort), nil))
		}()
	}
}
