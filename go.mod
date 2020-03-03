module github.com/openshift/image-registry

go 1.13

require (
	bitbucket.org/ww/goautoneg v0.0.0-20120707110453-75cd24fc2f2c
	github.com/Azure/azure-sdk-for-go v16.2.1+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.9.3 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.8.1 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/aws/aws-sdk-go v1.28.2 // indirect
	github.com/bshuster-repo/logrus-logstash-hook v0.4.1
	github.com/denverdino/aliyungo v0.0.0-20161108032828-afedced274aa // indirect
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/distribution v0.0.0-20180920194744-16128bbac47f
	github.com/docker/docker v1.4.2-0.20170731201646-1009e6a40b29
	github.com/docker/go-units v0.4.0
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7
	github.com/garyburd/redigo v0.0.0-20150301180006-535138d7bcd7 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache v0.0.0-20191002201903-404acd9df4cc // indirect
	github.com/gorilla/context v0.0.0-20140604161150-14f550f51af5 // indirect
	github.com/gorilla/handlers v0.0.0-20150720190736-60c7bfde3e33
	github.com/gorilla/mux v1.4.0 // indirect
	github.com/hashicorp/golang-lru v0.5.3
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/marstr/guid v1.1.1-0.20170427235115-8bdf7d1a087c // indirect
	github.com/ncw/swift v1.0.49 // indirect
	github.com/onsi/ginkgo v1.10.2 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/runc v1.0.0-rc5.0.20180920170208-00dc70017d22 // indirect
	github.com/openshift/api v0.0.0-20200210091934-a0e53e94816b
	github.com/openshift/client-go v0.0.0-20200116152001-92a2713fa240
	github.com/openshift/library-go v0.0.0-20200226171210-caa110959f91
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/procfs v0.0.8 // indirect
	github.com/satori/go.uuid v1.2.1-0.20180103174451-36e9d2ebbde5 // indirect
	github.com/sirupsen/logrus v1.4.2
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543 // indirect
	google.golang.org/cloud v0.0.0-20151119220103-975617b05ea8 // indirect
	gopkg.in/yaml.v2 v2.2.7
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.1
	k8s.io/klog v1.0.0
)

replace github.com/docker/distribution => github.com/openshift/docker-distribution v2.5.0-rc.1.0.20200110114316-95666ed3a0e2+incompatible

replace google.golang.org/api => google.golang.org/api v0.0.0-20160322025152-9bf6e6e569ff
