module github.com/openshift/image-registry

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v16.2.1+incompatible // indirect
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/aws/aws-sdk-go v1.34.30 // indirect
	github.com/bshuster-repo/logrus-logstash-hook v0.4.1
	github.com/containerd/containerd v1.3.3 // indirect
	github.com/denverdino/aliyungo v0.0.0-20161108032828-afedced274aa // indirect
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/distribution v0.0.0-20180920194744-16128bbac47f
	github.com/docker/docker v1.4.2-0.20200229013735-71373c6105e3
	github.com/docker/go-units v0.4.0
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7
	github.com/garyburd/redigo v0.0.0-20150301180006-535138d7bcd7 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/gorilla/context v0.0.0-20140604161150-14f550f51af5 // indirect
	github.com/gorilla/handlers v0.0.0-20150720190736-60c7bfde3e33
	github.com/gorilla/mux v1.4.0 // indirect
	github.com/hashicorp/golang-lru v0.5.3
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/marstr/guid v1.1.1-0.20170427235115-8bdf7d1a087c // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
	github.com/ncw/swift v1.0.49 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc5.0.20180920170208-00dc70017d22 // indirect
	github.com/openshift/api v0.0.0-20201019163320-c6a5ec25f267
	github.com/openshift/client-go v0.0.0-20201020074620-f8fd44879f7c
	github.com/openshift/library-go v0.0.0-20200921120329-c803a7b7bb2c
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.7.1
	github.com/satori/go.uuid v1.2.1-0.20180103174451-36e9d2ebbde5 // indirect
	github.com/sirupsen/logrus v1.6.0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	google.golang.org/cloud v0.0.0-20151119220103-975617b05ea8 // indirect
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v0.19.2
	k8s.io/klog/v2 v2.3.0
)

replace (
	github.com/docker/distribution => github.com/openshift/docker-distribution v0.0.0-20201012083032-63d38f155de2
	github.com/openshift/library-go => github.com/kevinrizza/library-go v0.0.0-20201216155812-db26a7547735
	google.golang.org/api => google.golang.org/api v0.0.0-20160322025152-9bf6e6e569ff
)
