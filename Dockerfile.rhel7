FROM registry.svc.ci.openshift.org/ocp/builder:golang-1.11 AS builder
WORKDIR /go/src/github.com/openshift/image-registry
COPY . .
RUN hack/build-go.sh

FROM registry.svc.ci.openshift.org/ocp/4.0:base
RUN yum install -y rsync
COPY --from=builder /go/src/github.com/openshift/image-registry/_output/local/bin/dockerregistry /usr/bin/
COPY images/dockerregistry/config.yml /
RUN chmod a+w -R /etc/pki/ca-trust/extracted
USER 1001
EXPOSE 5000
VOLUME /registry
ENV REGISTRY_CONFIGURATION_PATH=/config.yml
ENTRYPOINT ["sh", "-c", "update-ca-trust && \"$@\"", "arg0"]
CMD ["/usr/bin/dockerregistry"]
LABEL io.k8s.display-name="OpenShift Image Registry" \
      io.k8s.description="This is a component of OpenShift and exposes a container image registry that is integrated with the cluster for authentication and management." \
      io.openshift.tags="openshift,docker,registry"
