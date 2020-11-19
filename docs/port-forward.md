`bosun kube port-forward` can be spelled `bosun k pf`

`bosun kube port-forward daemon` This starts a daemon that you leave running as long as you want your port-forwards to be managed for
`bosun kube port-forward add {name}` This opens an editor where you can configure a port-forward. It looks like this:
```
active: true 
localPort: 27418     # port you want to use on your machine
kubeConfig: ""         # path to kubeconfig to use if not the default
kubeContext: ""      # kube context to use (oci-prod-01, oci-uat, etc)
targetType: ""          # svc or pod, defaults to pod
targetName: ""        # name of service or pod
targetPort: 27017  # port on service or pod
namespace: default # namespace of target
args: [] # set to an args array if you want to just put in the same args you'd pass to kubectl port-forward, instead of using the values above
```

The daemon will create that port-forward and keep it running until you stop the daemon or do `bosun kube port-forward stop [name]`. This will restart it even if the pod gets recycled or the OCI k8s integration flakes out and kills it.

You can start port-forwards with `bosun kube port-forward start [name]` and edit them with  `bosun kube port-forward edit [name]`.

If you don't provide `[name]` in a command it will ask you which one you want.

`bosun kube port-forward ls` will show your current port forwards:
```
+--------------------+--------+---------+--------------------------------------------------------------------------------------------------+-------+
| NAME               | ACTIVE | STATE   | CONFIG                                                                                           | ERROR |
+--------------------+--------+---------+--------------------------------------------------------------------------------------------------+-------+
| preprod-auth-mongo | true   | Running | port-forward --namespace default --context oci-preprod-auth mongodb-0 27418:27017                |       |
| preprod-cassandra  | true   | Running | port-forward --namespace default --context oci-preprod cassandra-0 49042:9042                    |       |
| preprod-elastic    | false  |         | port-forward --namespace default --context oci-preprod pod/elasticsearch-es-default-1 49200:9200 |       |
| preprod-mongo      | true   | Running | port-forward --namespace default mongodb-1 27417:27017                                           |       |
+--------------------+--------+---------+--------------------------------------------------------------------------------------------------+-------+
```

If there is an error or some problem with the port forward it'll be in the error column.

Logs for the daemon are in `~/.bosun/port-forwards/daemon.log`
Other related files are in that directory too.

Enjoy.