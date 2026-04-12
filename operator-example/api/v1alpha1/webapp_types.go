package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebAppSpec: 사용자가 선언하는 "원하는 상태(Desired State)"
type WebAppSpec struct {
	// 실행할 컨테이너 이미지
	Image string `json:"image"`

	// 유지할 파드 수
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	Replicas int32 `json:"replicas,omitempty"`

	// 컨테이너가 수신할 포트
	// +kubebuilder:default=8080
	Port int32 `json:"port,omitempty"`
}

// WebAppStatus: 컨트롤러가 기록하는 "현재 상태(Observed State)"
type WebAppStatus struct {
	// 현재 Ready 상태인 파드 수
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// 현재 상태를 설명하는 조건 목록
	// 예: Available=True, Progressing=True
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WebApp은 Deployment + Service를 하나의 리소스로 추상화합니다.
type WebApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebAppSpec   `json:"spec,omitempty"`
	Status WebAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebAppList는 WebApp의 목록입니다.
type WebAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebApp{}, &WebAppList{})
}
