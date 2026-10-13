# How to deploy development build of image registry

## Prerequisites

 * An OpenShift cluster.
 * Credentials from [the app.ci cluster](https://console-openshift-console.apps.ci.l2s4.p1.openshiftapps.com/).
 * A public image repository (for example, you can create a public repository on [quay.io](https://quay.io/)).

## Logging into the app.ci cluster and its registry

1. Copy the login command from <https://console-openshift-console.apps.ci.l2s4.p1.openshiftapps.com/> and run it
2. Rename the context for the `app.ci` cluster

    ```
    oc config rename-context "$(oc config current-context)" app.ci
    ```

3. Login into the registry `registry.ci.openshift.org`

    ```
    oc --context=app.ci whoami -t | docker login -u unused --password-stdin "$(oc --context=app.ci registry info --public=true)"
    ```

## Switching the image registry operator into the Unmanaged mode

1. You'll need to change the image registry deployment manually, so disable the image registry operator

    ```
    oc patch configs.imageregistry cluster --type=merge -p '{"spec":{"managementState":"Unmanaged"}}'
    ```

## Building and deploying a new container image

1. Go to the directory with the image registry sources

    ```
    cd ./openshift/image-registry
    ```

2. Build a new image

    ```
    docker build -f Dockerfile.rhel7 -t quay.io/rh-obulatov/image-registry .
    ```

3. Push the new image

    ```
    docker push quay.io/rh-obulatov/image-registry
    ```

4. Deploy the new build

    ```
    oc -n openshift-image-registry set image deploy/image-registry registry="$(docker inspect --format='{{index .RepoDigests 0}}' quay.io/rh-obulatov/image-registry)"
    ```

5. Wait until the pods use the new image

    ```
    oc -n openshift-image-registry get pods -l docker-registry=default -o custom-columns="NAME:.metadata.name,STATUS:.status.phase,IMAGE:.spec.containers[0].image"
    ```

6. Enjoy your registry!
