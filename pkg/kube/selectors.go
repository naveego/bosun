package kube

import "github.com/naveego/bosun/pkg/core"

type ClusterRoleGetter interface {
	GetClusterRole() core.ClusterRole
}
