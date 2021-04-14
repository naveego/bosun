package brns

import (
	"fmt"
	"github.com/pkg/errors"
	"strings"
)

type EnvironmentBrn struct {
	EnvironmentName string
}

type ClusterBrn struct {
	EnvironmentBrn
	ClusterName     string
}

func (s EnvironmentBrn) String() string {
	return s.EnvironmentName
}

func (s ClusterBrn) String() string {
	brn := fmt.Sprintf("%s:%s", s.EnvironmentBrn, s.ClusterName)
	return brn
}

type StackBrn struct {
	ClusterBrn
	StackName string
}

func (s StackBrn) IsEmpty() bool {
	return s.EnvironmentName == "" && s.ClusterName == "" && s.StackName == ""
}

func (s StackBrn) String() string {
	brn := strings.TrimSuffix(fmt.Sprintf("%s/%s", s.ClusterBrn, s.StackName), "/")
	return brn
}

func (s StackBrn) Equals(brn StackBrn) bool {
	return s.EnvironmentName == brn.EnvironmentName &&
		s.ClusterName == brn.ClusterName &&
		s.StackName == brn.StackName
}

func NewEnvironment(env string) EnvironmentBrn {
	return EnvironmentBrn{EnvironmentName: env}
}

func NewCluster(env string, cluster string) ClusterBrn {
	return ClusterBrn{EnvironmentBrn: NewEnvironment(env), ClusterName: cluster}
}

func NewStack(env string, cluster string, stack string) StackBrn {
	return StackBrn{ClusterBrn: NewCluster(env, cluster), StackName: stack}
}

type BrnParts struct {
	Raw string
	Environment string
	EnvironmentOrCluster string
	Cluster string
	Stack string
}

func TryParseStack(raw string) BrnParts {
	brnParts := BrnParts{
		Raw: raw,
	}
	var clusterStack string

	if strings.Contains(raw, ":"){
		parts := strings.Split(raw, ":")
		brnParts.Environment = parts[0]
		clusterStack = parts[1]
	} else {
		clusterStack = raw
	}

	if clusterStack != "" {
		if strings.Contains(clusterStack, "/") {
			parts := strings.Split(clusterStack, "/")
			brnParts.Cluster = parts[0]
			brnParts.Stack = parts[1]
		} else {
			brnParts.EnvironmentOrCluster = clusterStack
			brnParts.Cluster = clusterStack
		}
	}
	return brnParts
}

func ParseStack(raw string) (StackBrn, error) {

	parts := TryParseStack(raw)


	if parts.Environment == "" && parts.Cluster == "" && parts.Stack == "" {
		return StackBrn{}, errors.Errorf("invalid stack brn %q, should be env:cluster/stack", raw)
	}

	return NewStack(parts.Environment, parts.Cluster, parts.Stack), nil
}