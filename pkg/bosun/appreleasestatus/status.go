package appreleasestatus

import "fmt"

type Status struct {
	Kind   string `yaml:"kind"`
	Reason string `yaml:"reason,omitempty"`
	Bump   string `yaml:"bump,omitempty"`
}

func (s Status) IsCandidate() bool {
	return s.Kind == KindCandidate
}

func (s Status) IsUpgrade() bool {
	return s.Kind == KindUpgrade
}
func (s Status) IsAvailable() bool {
	return s.Kind == KindAvailable
}

func (s Status) String() string {
	return fmt.Sprintf("%s")
}

func Candidate(reason string, args ...interface{}) Status {
	return New(KindCandidate, reason, args...)
}

func Upgrade(bump string, reason string, args ...interface{}) Status {
	s := New(KindUpgrade, reason, args...)
	s.Bump = bump
	return s
}

func Deploy(reason string, args ...interface{}) Status {
	return New(KindDeploy, reason, args...)
}

func Available(reason string, args ...interface{}) Status {
	return New(KindAvailable, reason, args...)
}

func New(kind string, reason string, args ...interface{}) Status {
	return Status{
		Kind:   kind,
		Reason: fmt.Sprintf(reason, args...),
	}
}

var StatusArr = []string{
	KindCandidate,
	KindUpgrade,
	KindDeploy,
	KindAvailable,
}

const (
	KindCandidate = "candidate"
	KindUpgrade   = "upgrade"
	KindDeploy    = "deploy"
	KindAvailable = "available"
)
