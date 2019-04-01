


# Bosun

CLI documentation is here: [./docs/bosun.md](./docs/bosun.md)

A commented example bosun.yaml file is here: [./examples/bosun.yaml](./examples/bosun.yaml)


## Quick Start

1. Install the dependencies below.
2. Copy ./examples/bosun.yaml to `$HOME/.bosun/bosun.yaml`.
3. Get the latest version of https://github.com/naveegoinc/devops, branch 2018.2.1. 
4. `go install github.com/naveego/bosun`
5. Run `$(bosun env red)` to set the environment variables for the red environment. Run it without the `$()` to see what it does.
6. Run `bosun script list` to make sure everything is registered. You should see one script, named `up`.
7. Run `docker stop $(docker ps -q)` to stop all your docker containers.
8. Clear any hardcoded *.n5o.red entries out of `etc/hosts`.
9. Run `bosun script up --verbose` to bring up minikube and deploy everything to it. 
   - You may need to run this a few times if things are slow to come up and subsequent steps time out.
   - After minikube has started you can run `minikube dashboard` to open the dashboard and see what things have been deployed.
   - After traefik is up (in the kube-system namespace) you can browse to http://traefik.n5o.red to see its dashboard.
   - You can browse to things routed through traefik using https if you install the certs in the ./dev/certs folder in the devops repo.
   
### Dependencies

- Docker (https://docs.docker.com/v17.12/install/)
- Go (https://golang.org/doc/install)
- Virtualbox (https://www.virtualbox.org/wiki/Downloads) ***
- Minikube (https://github.com/kubernetes/minikube)
- Kubernetes (https://kubernetes.io/)
- Vault (https://www.vaultproject.io/docs/install/index.html)
- Helm >v2.11.0 (https://github.com/helm/helm) 
  - Helm diff plugin (https://github.com/databus23/helm-diff)
  - Helm s3 plugin (https://github.com/hypnoglow/helm-s3)
- LastPass CLI (https://github.com/lastpass/lastpass-cli) 
    - To avoid storing passwords in scripts, only needed if you're touching the blue environment.   


*** https://askubuntu.com/questions/465454/problem-with-the-installation-of-virtualbox

## How to make microservices available as apps

1. Add a `routeToHost: false` entry in the values.yaml file for your chart.
2. Make the spec in the service for your chart look like this:
```yaml
spec:
{{- if .Values.routeToHost }}
  clusterIP: # must be empty to delete clusterIP assigned by kube
  type: ExternalName
  # This is a DNS record that points to an IP that resolves to your physical computer
  # from within minikube. That should be 192.168.99.1 in virtualbox.
  externalName: minikube-host.n5o.red 
  ports:
    - port: # the port your service listens to when running on localhost 
      targetPort:  # the port your service listens to when running on localhost 
      protocol: TCP
      name: http
{{- else }}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: http
      protocol: TCP
      name: http
  type: {{ .Values.service.type }}
  selector:
    app: {{ template "YOUR_MICROSERVICE.name" . }}
    release: {{ .Release.Name }}
{{- end}}

``` 
3. Add a `bosun.yaml` file in your microservice:
```yaml
apps:
  - name: MICROSERVICE_NAME
    version: MICROSERVICE_VERSION
    repo: naveegoinc/REPO
    chartPath: deploy/charts/MICROSERVICE_NAME #this should be a relative path from the bosun.yaml file
    runCommand: [ ... ] # a command that will start your microservice on your machine
```
3. Run `bosun config add {path to bosun file you just created}`.
4. Run `bosun app list`. You should see your app on the list.
5. Run `bosun app run {your microservice name}`. Your microservice should start and you should be able to open it in the browser.
6. Run `bosun app toggle --minikube`. Your microservice will now be served from a container in minikube.
