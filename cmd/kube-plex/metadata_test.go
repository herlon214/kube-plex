package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-test/deep"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_pmsMetadata_FetchMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	validPod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "plex", Name: "pms", UID: "123",
			Annotations: map[string]string{"kube-plex/pms-addr": "service:32400"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "plex", Image: "plex:test"}},
			Volumes:    []corev1.Volume{{Name: "data"}, {Name: "transcode"}},
		},
		Status: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{{Name: "kube-plex-init", Image: "kubeplex:latest", ImageID: "kubeplex@sha256:12345"}},
			ContainerStatuses:     []corev1.ContainerStatus{{Name: "plex", Image: "pms:latest", ImageID: "pms@sha256:12345"}},
		},
	}

	tests := []struct {
		name         string
		podname      string
		podnamespace string
		pod          corev1.Pod
		wantPms      PmsMetadata
		wantErr      bool
	}{
		{"fetches info from api", "pms", "plex", validPod, PmsMetadata{Name: "pms", Namespace: "plex", UID: "123", PmsImage: "pms@sha256:12345", KubePlexImage: "kubeplex@sha256:12345", PmsAddr: "service:32400", Volumes: []corev1.Volume{{Name: "data"}, {Name: "transcode"}}}, false},
		{"fails on missing podname", "", "plex", validPod, PmsMetadata{}, true},
		{"fails on missing namespace", "pms", "", validPod, PmsMetadata{}, true},
		{"fails gracefully on wrong pod name", "wrong", "plex", validPod, PmsMetadata{}, true},
		{"fails gracefully on wrong namespace", "pms", "wrong", validPod, PmsMetadata{}, true},
		{"plex container missing", "pms", "plex", corev1.Pod{ObjectMeta: validPod.ObjectMeta, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "wrong", Image: "pms:own"}}, Volumes: []corev1.Volume{{Name: "data"}, {Name: "transcode"}}}}, PmsMetadata{}, true},
		{"plex data volume missing", "pms", "plex", corev1.Pod{ObjectMeta: validPod.ObjectMeta, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "plex", Image: "pms:own"}}, Volumes: []corev1.Volume{{Name: "transcode"}}}}, PmsMetadata{}, true},
		{"plex transcode volume missing", "pms", "plex", corev1.Pod{ObjectMeta: validPod.ObjectMeta, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "plex", Image: "pms:own"}}, Volumes: []corev1.Volume{{Name: "data"}}}}, PmsMetadata{}, true},
		{"kube-plex debug set", "pms", "plex", corev1.Pod{ObjectMeta: v1.ObjectMeta{Namespace: "plex", Name: "pms", UID: "123", Annotations: map[string]string{"kube-plex/pms-addr": "a:32400", "kube-plex/loglevel": "debug"}}, Spec: validPod.Spec, Status: validPod.Status}, PmsMetadata{Name: "pms", Namespace: "plex", UID: "123", PmsImage: "pms@sha256:12345", KubePlexImage: "kubeplex@sha256:12345", KubePlexLevel: "debug", PmsAddr: "a:32400", Volumes: []corev1.Volume{{Name: "data"}, {Name: "transcode"}}}, false},
		{"renamed kube-plex container", "pms", "plex",
			corev1.Pod{ObjectMeta: v1.ObjectMeta{Namespace: "plex", Name: "pms", UID: "123", Annotations: map[string]string{"kube-plex/container-name": "kp-init", "kube-plex/pms-addr": "a:32400"}}, Spec: validPod.Spec, Status: corev1.PodStatus{ContainerStatuses: validPod.Status.ContainerStatuses, InitContainerStatuses: []corev1.ContainerStatus{{Name: "kp-init", ImageID: "aaa@sha256:12345"}}}},
			PmsMetadata{Name: "pms", Namespace: "plex", UID: "123", PmsImage: "pms@sha256:12345", KubePlexImage: "aaa@sha256:12345", PmsAddr: "a:32400", Volumes: []corev1.Volume{{Name: "data"}, {Name: "transcode"}}}, false,
		},
		{"renamed PMS container", "pms", "plex",
			corev1.Pod{ObjectMeta: v1.ObjectMeta{Namespace: "plex", Name: "pms", UID: "123", Annotations: map[string]string{"kube-plex/pms-container-name": "test", "kube-plex/pms-addr": "a:32400"}}, Spec: validPod.Spec, Status: corev1.PodStatus{InitContainerStatuses: validPod.Status.InitContainerStatuses, ContainerStatuses: []corev1.ContainerStatus{{Name: "test", ImageID: "aaa@sha256:12345"}}}},
			PmsMetadata{Name: "pms", Namespace: "plex", UID: "123", PmsImage: "aaa@sha256:12345", KubePlexImage: "kubeplex@sha256:12345", PmsAddr: "a:32400", Volumes: []corev1.Volume{{Name: "data"}, {Name: "transcode"}}}, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewSimpleClientset(&tt.pod)
			m, err := FetchMetadata(ctx, cl, tt.podname, tt.podnamespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("pmsMetadata.FetchAPI() error = %v, wantErr %v", err, tt.wantErr)
			}
			// We don't want to check output state if error occurs
			if !tt.wantErr {
				if diff := deep.Equal(m, tt.wantPms); diff != nil {
					t.Errorf("pmsMetadata.FetchAPI() diff: %v", diff)
				}
			}
		})
	}
}

func Test_pmsMetadata_OwnerReference(t *testing.T) {
	tests := []struct {
		name    string
		obj     PmsMetadata
		want    v1.OwnerReference
		wantErr bool
	}{
		{"success", PmsMetadata{Name: "testpod", Namespace: "plex", UID: "123"}, v1.OwnerReference{APIVersion: "v1", Kind: "Pod", Name: "testpod", UID: "123"}, false},
		{"missing uuid", PmsMetadata{Name: "testpod", Namespace: "plex"}, v1.OwnerReference{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.obj
			got, err := p.OwnerReference()
			if (err != nil) != tt.wantErr {
				t.Errorf("pmsMetadata.OwnerReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pmsMetadata.OwnerReference() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_pmsMetadata_LauncherCmd(t *testing.T) {
	tests := []struct {
		name string
		p    PmsMetadata
		args []string
		want []string
	}{
		{"generates bare cmd", PmsMetadata{PmsAddr: "a:32400"}, []string{"a"}, []string{"/shared/transcode-launcher", "--pms-addr=a:32400", "--listen=:32400", "--", "a"}},
		{"generates codec server url", PmsMetadata{PmsAddr: "a:32400", PodIP: "1.2.3.4", CodecPort: 1234}, []string{"a"}, []string{"/shared/transcode-launcher", "--pms-addr=a:32400", "--listen=:32400", "--codec-server-url=http://1.2.3.4:1234/", "--codec-dir=/shared/codecs/", "--", "a"}},
		{"generates debug flag", PmsMetadata{PmsAddr: "a:32400", KubePlexLevel: "debug"}, []string{"a"}, []string{"/shared/transcode-launcher", "--pms-addr=a:32400", "--listen=:32400", "--loglevel=debug", "--", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.p
			if got := p.LauncherCmd(tt.args...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pmsMetadata.LauncherCmd() = %v, want %v", got, tt.want)
			}
		})
	}
}