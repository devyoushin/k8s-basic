// Package v1alpha1 은 WebApp CRD의 API 타입을 정의합니다.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion: CRD가 속하는 API 그룹과 버전
	// kubectl get webapp 시 사용하는 apiVersion: apps.example.com/v1alpha1
	GroupVersion = schema.GroupVersion{
		Group:   "apps.example.com",
		Version: "v1alpha1",
	}

	// SchemeBuilder: 타입을 runtime.Scheme에 등록하는 헬퍼
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme: main.go에서 호출하여 타입을 등록
	AddToScheme = SchemeBuilder.AddToScheme
)
