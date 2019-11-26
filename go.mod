module github.com/openshift/image-registry

go 1.13

require (
	bitbucket.org/ww/goautoneg v0.0.0-20120707110453-75cd24fc2f2c
	github.com/Azure/azure-sdk-for-go v16.2.1+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.9.2 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/aws/aws-sdk-go v1.17.14 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bshuster-repo/logrus-logstash-hook v0.4.1
	github.com/certifi/gocertifi v0.0.0-20180905225744-ee1a9a0726d2 // indirect
	github.com/denverdino/aliyungo v0.0.0-20161108032828-afedced274aa // indirect
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/distribution v0.0.0-00010101000000-000000000000
	github.com/docker/docker v1.4.2-0.20170731201646-1009e6a40b29
	github.com/docker/go-connections v0.3.0 // indirect
	github.com/docker/go-metrics v0.0.0-20180209012529-399ea8c73916 // indirect
	github.com/docker/go-units v0.3.3
	github.com/docker/libtrust v0.0.0-20150114040149-fa567046d9b1
	github.com/garyburd/redigo v0.0.0-20150301180006-535138d7bcd7 // indirect
	github.com/getsentry/raven-go v0.0.0-20171206001108-32a13797442c // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache v0.0.0-20191002201903-404acd9df4cc // indirect
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/gorilla/context v0.0.0-20140604161150-14f550f51af5 // indirect
	github.com/gorilla/handlers v0.0.0-20150720190736-60c7bfde3e33
	github.com/gorilla/mux v1.4.0 // indirect
	github.com/hashicorp/golang-lru v0.5.3
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/marstr/guid v1.1.1-0.20170427235115-8bdf7d1a087c // indirect
	github.com/mitchellh/mapstructure v0.0.0-20150528213339-482a9fd5fa83 // indirect
	github.com/ncw/swift v1.0.49 // indirect
	github.com/onsi/ginkgo v1.10.2 // indirect
	github.com/onsi/gomega v1.7.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/image-spec v1.0.1-0.20180918080442-7b1e489870ac // indirect
	github.com/opencontainers/runc v1.0.0-rc5.0.20180920170208-00dc70017d22 // indirect
	github.com/openshift/api v3.9.1-0.20191023095241-48e39eee5d1f+incompatible
	github.com/openshift/client-go v0.0.0-20191022152013-2823239d2298
	github.com/openshift/library-go v0.0.0-20191023092337-10b6237962f7
	github.com/pborman/uuid v1.2.0
	github.com/pkg/profile v1.2.2-0.20180809112205-057bc52a47ec // indirect
	github.com/prometheus/client_golang v0.9.4
	github.com/prometheus/procfs v0.0.5 // indirect
	github.com/satori/go.uuid v1.2.1-0.20180103174451-36e9d2ebbde5 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/net v0.0.0-20191021144547-ec77196f6094 // indirect
	google.golang.org/cloud v0.0.0-20151119220103-975617b05ea8 // indirect
	google.golang.org/grpc v1.23.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.2.4
	k8s.io/api v0.0.0-20191016110408-35e52d86657a
	k8s.io/apimachinery v0.0.0-20191004115801-a2eda9f80ab8
	k8s.io/apiserver v0.0.0-20191016112112-5190913f932d // indirect
	k8s.io/client-go v0.0.0-20191016111102-bec269661e48
	k8s.io/klog v1.0.0
)

replace github.com/docker/distribution => github.com/openshift/docker-distribution v2.5.0-rc.1.0.20190226150947-f975179b0b58+incompatible

replace google.golang.org/api => google.golang.org/api v0.0.0-20160322025152-9bf6e6e569ff
