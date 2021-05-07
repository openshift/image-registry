module github.com/openshift/image-registry

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v54.0.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/aws/aws-sdk-go v1.38.35 // indirect
	github.com/bshuster-repo/logrus-logstash-hook v0.4.1
	github.com/containerd/containerd v1.3.3 // indirect
	github.com/denverdino/aliyungo v0.0.0-20161108032828-afedced274aa // indirect
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/distribution v0.0.0-20180920194744-16128bbac47f
	github.com/docker/docker v1.4.2-0.20200229013735-71373c6105e3
	github.com/docker/go-units v0.4.0
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7
	github.com/garyburd/redigo v0.0.0-20150301180006-535138d7bcd7 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/gorilla/context v0.0.0-20140604161150-14f550f51af5 // indirect
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.4.0 // indirect
	github.com/hashicorp/golang-lru v0.5.3
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
	github.com/ncw/swift v1.0.49 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc5.0.20180920170208-00dc70017d22 // indirect
	github.com/openshift/api v0.0.0-20200827090112-c05698d102cf
	github.com/openshift/client-go v0.0.0-20200827190008-3062137373b5
	github.com/openshift/library-go v0.0.0-20200921120329-c803a7b7bb2c
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.7.1
	github.com/sirupsen/logrus v1.7.0
	golang.org/x/crypto v0.0.0-20210506145944-38f3c27a63bf
	google.golang.org/cloud v0.0.0-20151119220103-975617b05ea8 // indirect
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/apiserver v0.21.0 // indirect
	k8s.io/client-go v0.21.0
	k8s.io/klog/v2 v2.8.0
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e // indirect
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.1.1 // indirect
)

replace (
	github.com/docker/distribution => github.com/openshift/docker-distribution v0.0.0-20210209103229-0526a4ae41e1
	google.golang.org/api => google.golang.org/api v0.0.0-20160322025152-9bf6e6e569ff
)
