# Bosun

CLI documentation is here: [./docs/bosun.md](./docs/bosun.md)

A commented example bosun.yaml file is here: [./examples/bosun.yaml](./examples/bosun.yaml)

> The Github Action for bosun is defined in the [./github-action](./github-action) folder.

## Quick Start

1. Install Go 1.11.9 from https://golang.org/doc/install
2. In the root of this repo, run `go install` (you might need to add `export GO11MODULES='on' in your .zshrc`)
3. Get the latest version of https://github.com/naveegoinc/devops (recommendation: clone to `$HOME/src/github.com/naveegoinc/devops`).
4. In the root of the devops repo, run `bosun workspace add bosun.yaml`
    - Run `bosun app list` to check if bosun has added all the imports to your workspace. It should list a bunch of apps.
    - If you don't see a bunch of apps, run `bosun workspace tidy`.    
    
5. Run `eval $(bosun env use red)` to set the environment variables for the red environment. Run it without the `$()` to see what it does.
    - You can add an alias to your .zshrc to make switching environments easier:
      ```
      function be() {
        eval $(bosun env use $1)
      }
      ```
      This allows you to write `> be qa` to switch to the QA environment, for example.
    - If you work in multiple environments it's a good idea to add `$BOSUN_ENVIRONMENT` to your prompt so it's always obvious where you are.

6. Run `bosun tools list` to see tools which are registered with bosun.
    - You'll need to install some of these tools to proceed, using `bosun tool install {name}`
    - virtualbox
    - minikube 
    - helm (https://helm.sh/docs/using_helm/#from-script) & helm diff plugin (https://github.com/databus23/helm-diff)
    - vault
    - mkcert
    - awscli
    - docker (must be installed manually right now, following instructions from https://docs.docker.com/install/linux/docker-ce/ubuntu/)
7. Add docker logins:
    - For our private repo: `sudo docker login docker.n5o.black`. Get username/password from your mentor.
    - For github: `sudo docker login docker.pkg.github.com` - The username is anything, the password is a github token with read:packages and write:packages.
    - After adding docker logins, make the docker config readable by bosun using `sudo chmod 700 ~/.docker && sudo chmod 600 ~/.docker/config.json`
8. Add aws login for CLI: `aws configure --profile black`. Get key/secret from your mentor.
9. Run `mkcert --install` if you haven't already
10. Run `bosun script up --verbose` to bring up minikube and deploy everything to it.
   - You may need to run this a few times if things are slow to come up and subsequent steps time out.
   - After minikube has started you can run `minikube dashboard` to open the dashboard and see what things have been deployed.
   - After traefik is up (in the kube-system namespace) you can browse to http://traefik.n5o.red to see its dashboard.
   - You can browse to things routed through traefik using https if you install the certs in the ./dev/certs folder in the devops repo.

### Troubleshooting

- **Docker Config Permission Denied**

  - Error:
    `error reading docker config from "/home/$USER/.docker/config.json": open /home/$USER/.docker/config.json: permission denied`
  - Solution: `sudo chown "$USER":"$USER" /home/"$USER"/.docker -R && sudo chmod g+rwx "/home/$USER/.docker" -R` ( https://askubuntu.com/questions/747778/docker-warning-config-json-permission-denied )

- **Logging Namespace Not Found**

  - Error: `deploy failed: Error: failed to create resource: namespaces "logging" not foun`
  - Solution: `kubectl create -f $(bosun app repo-path devops)/logging-namespace.json`

- **No Chart.yaml in Helm Repo**
  - Error: `load default values from "": Error: no Chart.yaml exists in directory "/home/$USER/.helm/repository"`
  - Solution: In the root directory of the listed repo run `bosun a add bosun.yaml`

- **Virtualbox**  
  - https://askubuntu.com/questions/465454/problem-with-the-installation-of-virtualbox

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
    runCommand: [...] # a command that will start your microservice on your machine
```

3. Run `bosun config add {path to bosun file you just created}`.
4. Run `bosun app list`. You should see your app on the list.
5. Run `bosun app run {your microservice name}`. Your microservice should start and you should be able to open it in the browser.
6. Run `bosun app toggle --minikube`. Your microservice will now be served from a container in minikube.
