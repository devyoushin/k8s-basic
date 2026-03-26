### k8s가 관리하는 리소스 외 kubectl get all -A 모든 리소스 종류(Resource Types) 목록 확인하는 방법

```bash
[root@ip-10-10-10-10 ~]# kubectl api-resources
NAME                                SHORTNAMES   APIVERSION                        NAMESPACED   KIND
bindings                                         v1                                true         Binding
componentstatuses                   cs           v1                                false        ComponentStatus
configmaps                          cm           v1                                true         ConfigMap
endpoints                           ep           v1                                true         Endpoints
events                              ev           v1                                true         Event
limitranges                         limits       v1                                true         LimitRange
namespaces                          ns           v1                                false        Namespace
nodes                               no           v1                                false        Node
persistentvolumeclaims              pvc          v1                                true         PersistentVolumeClaim
persistentvolumes                   pv           v1                                false        PersistentVolume
pods                                po           v1                                true         Pod
podtemplates                                     v1                                true         PodTemplate
replicationcontrollers              rc           v1                                true         ReplicationController
resourcequotas                      quota        v1                                true         ResourceQuota
secrets                                          v1                                true         Secret
serviceaccounts                     sa           v1                                true         ServiceAccount
services                            svc          v1                                true         Service
mutatingwebhookconfigurations                    admissionregistration.k8s.io/v1   false        MutatingWebhookConfiguration
validatingadmissionpolicies                      admissionregistration.k8s.io/v1   false        ValidatingAdmissionPolicy
validatingadmissionpolicybindings                admissionregistration.k8s.io/v1   false        ValidatingAdmissionPolicyBinding
validatingwebhookconfigurations                  admissionregistration.k8s.io/v1   false        ValidatingWebhookConfiguration
customresourcedefinitions           crd,crds     apiextensions.k8s.io/v1           false        CustomResourceDefinition
apiservices                                      apiregistration.k8s.io/v1         false        APIService
controllerrevisions                              apps/v1                           true         ControllerRevision
daemonsets                          ds           apps/v1                           true         DaemonSet
deployments                         deploy       apps/v1                           true         Deployment
replicasets                         rs           apps/v1                           true         ReplicaSet
statefulsets                        sts          apps/v1                           true         StatefulSet
selfsubjectreviews                               authentication.k8s.io/v1          false        SelfSubjectReview
tokenreviews                                     authentication.k8s.io/v1          false        TokenReview
localsubjectaccessreviews                        authorization.k8s.io/v1           true         LocalSubjectAccessReview
selfsubjectaccessreviews                         authorization.k8s.io/v1           false        SelfSubjectAccessReview
selfsubjectrulesreviews                          authorization.k8s.io/v1           false        SelfSubjectRulesReview
subjectaccessreviews                             authorization.k8s.io/v1           false        SubjectAccessReview
horizontalpodautoscalers            hpa          autoscaling/v2                    true         HorizontalPodAutoscaler
cronjobs                            cj           batch/v1                          true         CronJob
jobs                                             batch/v1                          true         Job
certificatesigningrequests          csr          certificates.k8s.io/v1            false        CertificateSigningRequest
leases                                           coordination.k8s.io/v1            true         Lease
eniconfigs                                       crd.k8s.amazonaws.com/v1alpha1    false        ENIConfig
endpointslices                                   discovery.k8s.io/v1               true         EndpointSlice
nodediagnostics                                  eks.amazonaws.com/v1alpha1        false        NodeDiagnostic
events                              ev           events.k8s.io/v1                  true         Event
wasmplugins                                      extensions.istio.io/v1alpha1      true         WasmPlugin
flowschemas                                      flowcontrol.apiserver.k8s.io/v1   false        FlowSchema
prioritylevelconfigurations                      flowcontrol.apiserver.k8s.io/v1   false        PriorityLevelConfiguration
destinationrules                    dr           networking.istio.io/v1            true         DestinationRule
envoyfilters                                     networking.istio.io/v1alpha3      true         EnvoyFilter
gateways                            gw           networking.istio.io/v1            true         Gateway
proxyconfigs                                     networking.istio.io/v1beta1       true         ProxyConfig
serviceentries                      se           networking.istio.io/v1            true         ServiceEntry
sidecars                                         networking.istio.io/v1            true         Sidecar
virtualservices                     vs           networking.istio.io/v1            true         VirtualService
workloadentries                     we           networking.istio.io/v1            true         WorkloadEntry
workloadgroups                      wg           networking.istio.io/v1            true         WorkloadGroup
applicationnetworkpolicies          anp          networking.k8s.aws/v1alpha1       true         ApplicationNetworkPolicy
clusternetworkpolicies              cnp          networking.k8s.aws/v1alpha1       false        ClusterNetworkPolicy
clusterpolicyendpoints              cpe          networking.k8s.aws/v1alpha1       false        ClusterPolicyEndpoint
policyendpoints                                  networking.k8s.aws/v1alpha1       true         PolicyEndpoint
ingressclasses                                   networking.k8s.io/v1              false        IngressClass
ingresses                           ing          networking.k8s.io/v1              true         Ingress
networkpolicies                     netpol       networking.k8s.io/v1              true         NetworkPolicy
runtimeclasses                                   node.k8s.io/v1                    false        RuntimeClass
poddisruptionbudgets                pdb          policy/v1                         true         PodDisruptionBudget
clusterrolebindings                              rbac.authorization.k8s.io/v1      false        ClusterRoleBinding
clusterroles                                     rbac.authorization.k8s.io/v1      false        ClusterRole
rolebindings                                     rbac.authorization.k8s.io/v1      true         RoleBinding
roles                                            rbac.authorization.k8s.io/v1      true         Role
priorityclasses                     pc           scheduling.k8s.io/v1              false        PriorityClass
authorizationpolicies               ap           security.istio.io/v1              true         AuthorizationPolicy
peerauthentications                 pa           security.istio.io/v1              true         PeerAuthentication
requestauthentications              ra           security.istio.io/v1              true         RequestAuthentication
csidrivers                                       storage.k8s.io/v1                 false        CSIDriver
csinodes                                         storage.k8s.io/v1                 false        CSINode
csistoragecapacities                             storage.k8s.io/v1                 true         CSIStorageCapacity
storageclasses                      sc           storage.k8s.io/v1                 false        StorageClass
volumeattachments                                storage.k8s.io/v1                 false        VolumeAttachment
volumeattributesclasses             vac          storage.k8s.io/v1beta1            false        VolumeAttributesClass
telemetries                         telemetry    telemetry.istio.io/v1             true         Telemetry
cninodes                            cnd          vpcresources.k8s.aws/v1alpha1     false        CNINode
securitygrouppolicies               sgp          vpcresources.k8s.aws/v1beta1      true         SecurityGroupPolicy
```
