package brns

import (
	"fmt"
	"github.com/pkg/errors"
	"strings"
)

type Environment struct {
	EnvironmentName string
}

type Cluster struct {
	Environment
	ClusterName     string
}

func (s Environment) String() string {
	return s.EnvironmentName
}

func (s Cluster) String() string {
	brn := fmt.Sprintf("%s:%s", s.Environment, s.ClusterName)
	return brn
}

type Stack struct {
	Cluster
	StackName string
}

func (s Stack) IsEmpty() bool {
	return s.EnvironmentName == "" && s.ClusterName == "" && s.StackName == ""
}

func (s Stack) String() string {
	brn := strings.TrimSuffix(fmt.Sprintf("%s/%s", s.Cluster, s.StackName), "/")
	return brn
}

func NewEnvironment(env string) Environment {
	return Environment{EnvironmentName: env}
}

func NewCluster(env string, cluster string) Cluster {
	return Cluster{Environment: NewEnvironment(env), ClusterName: cluster}
}

func NewStack(env string, cluster string, stack string) Stack {
	return Stack{Cluster: NewCluster(env, cluster), StackName: stack}
}

func ParseStack(raw string) (Stack, error) {

	var env, cluster, stack string

	var clusterStack string

	if strings.Contains(raw, ":"){
		parts := strings.Split(raw, ":")
		env = parts[0]
		clusterStack = parts[1]
	} else {
		clusterStack = raw
	}

	if clusterStack != "" {
		if strings.Contains(clusterStack, "/") {
			parts := strings.Split(clusterStack, "/")
			cluster = parts[0]
			stack = parts[1]
		} else {
			cluster = clusterStack
		}
	}

	if env == "" && cluster == "" && stack == "" {
		return Stack{}, errors.Errorf("invalid stack brn %q, should be env:cluster/stack")
	}

	return NewStack(env, cluster, stack), nil
}