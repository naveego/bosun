// Copyright 2014 by James Dean Palmer and others.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

// Statecraft is a state machine engine for Go. State machines are
// composed of states and transitions between states.  While
// Statecraft can implement finite-state machines (FSM), it can also
// be used to devise machines where transitions are added over time or have
// guarded transitions which exhibit more complicated behavior.
//
// See also: http://bitbucket.org/jdpalmer/statecraft
package statecraft

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The Machine object represents states, transitions and actions for a
// single state machine.  The exported fields Src, Dst, and Evt are
// only defined when an action is being executed and contain,
// respectively, the source state, destination state, and event name.
type Machine struct {
	currentState    string
	transitions     map[string]string
	actions         map[string]func(Event)
	pendingEvent    Event
	processingEvent bool
	cancellable     bool
	cancel          bool
	Src             string
	Dst             string
	Evt             Event
}

func (m *Machine) reset() {
	m.processingEvent = false
	m.cancellable = false
	m.cancel = false
	m.pendingEvent = Event{}
	m.Src = ""
	m.Dst = ""
	m.Evt = Event{}
}

type Event struct {
	Name    string
	Payload interface{}
}

func (e Event) String() string {
	if e.Payload == nil {
		return e.Name
	}
	j, _ := json.Marshal(e.Payload)
	return e.Name + ": " + string(j)
}

func (e Event) Zero() bool {
	return e.Name == ""
}

// NewMachine - Create a new Machine with the specified initial state.
func NewMachine(initial string) *Machine {
	self := new(Machine)
	self.transitions = make(map[string]string)
	self.actions = make(map[string]func(Event))
	self.currentState = initial
	self.reset()
	return self
}

// OnEnterState evaluates the action as the machine enters state.
func (m *Machine) OnEnterState(state string, fn func(Event)) {
	m.Action(">"+state, fn)
}

// OnLeaveState evaluates the action as the machine leaves state.
func (m *Machine) OnLeaveState(state string, fn func(Event)) {
	m.Action("<"+state, fn)
}

// OnBeforeEvent evaluates the action before evt.
func (m *Machine) OnBeforeEvent(evt string, fn func(Event)) {
	m.Action(">>"+evt, fn)
}

// OnAfterEvent evaluates the action after evt.
func (m *Machine) OnAfterEvent(evt string, fn func(Event)) {
	m.Action("<<"+evt, fn)
}

// OnEventNotMatched evaluates the action after evt occurs in a state where no transition is defined.
func (m *Machine) OnEventNotMatched(evt string, fn func(Event)) {
	m.Action("!!"+evt, fn)
}

// OnUnhandled evaluates the action if no transition or error handler is defined for the event.
func (m *Machine) OnUnhandled(fn func(Event)) {
	m.Action("!!!", fn)
}

// Action Attach fn to the transition described in specifier.
//
// Specifiers use a special prefix minilanguage to annotate how the
// function should be attached to a transition.  Specifically:
//
//   >myState  - Evaluate the action as the machine enters myState.
//   <myState  - Evaluate the action as the machine leaves myState.
//   >*        - Evaluate the action before entering any state.
//   <*        - Evaluate the action after leaving any state.
//   >>myEvent - Evaluate the action before myEvent.
//   <<myEvent - Evaluate the action after myEvent.
//   >>*       - Evaluate the action before responding to any event.
//   <<*       - Evaluate the action after responding to any event.
//   !myState  - Evaluate the action if the machine is in myState when
//               an event is not matched.
//   !!myEvent - Evaluate the action if myEvent is not matched.
//   !*        - Evaluate the action for all states where an event is not
//               matched.
//   !!!       - Evaluate the action if and only if the match failed
//               and no other error handling code would be evaluated.
func (m *Machine) Action(specifier string, fn func(Event)) {
	m.actions[specifier] = fn
}

// Attempts to cancel an executing event.  If successful the function
// returns true and false otherwise.
func (m *Machine) Cancel() bool {
	if !m.cancellable {
		return false
	}
	m.cancel = true
	return true
}

// Fire an event which may cause the machine to change state.
func (m *Machine) Send(name string) {
	m.SendEvent(Event{Name: name})
}

// Fire an event with a payload which may cause the machine to change state.
func (m *Machine) SendPayload(name string, payload Event) {
	m.SendEvent(Event{Name: name, Payload: payload})
}

// Fire an event with a payload which may cause the machine to change state.
func (m *Machine) SendEvent(event Event) {
	if m.cancel {
		return
	}

	if m.processingEvent {
		m.pendingEvent = event
		return
	}

	m.Src = m.currentState
	m.Evt = event
	nextState, ok := m.transitions[event.Name+"_"+m.currentState]
	m.processingEvent = true

	if ok {
		m.Dst = nextState
		m.cancellable = true

		f := m.actions[">>"+event.Name]
		if f != nil {
			f(event)
		}
		if m.cancel {
			m.reset()
			return
		}
		f = m.actions[">>*"]
		if f != nil {
			f(event)
		}
		if m.cancel {
			m.reset()
			return
		}
		f = m.actions["<"+m.currentState]
		if f != nil {
			f(event)
		}
		if m.cancel {
			m.reset()
			return
		}
		f = m.actions["<*"]
		if f != nil {
			f(event)
		}
		if m.cancel {
			m.reset()
			return
		}
		f = m.actions[fmt.Sprintf("%s_%s_%s", event, m.currentState, nextState)]
		if f != nil {
			f(event)
		}
		if m.cancel {
			m.reset()
			return
		}

		m.currentState = nextState
		m.cancellable = false

		f = m.actions[">"+m.currentState]
		if f != nil {
			f(event)
		}
		f = m.actions[">*"]
		if f != nil {
			f(event)
		}
		f = m.actions["<<"+event.Name]
		if f != nil {
			f(event)
		}
		f = m.actions["<<*"]
		if f != nil {
			f(event)
		}

		m.Dst = ""
	} else {
		cnt := 0
		f := m.actions["!!"+event.Name]
		if f != nil {
			f(event)
			cnt += 1
		}
		f = m.actions["!"+m.currentState]
		if f != nil {
			f(event)
			cnt += 1
		}
		f = m.actions["!*"]
		if f != nil {
			f(event)
			cnt += 1
		}
		if cnt == 0 {
			f = m.actions["!!!"]
			if f != nil {
				f(event)
			}
		}
	}
	m.Src = ""
	m.Evt = Event{}

	m.processingEvent = false

	if !m.pendingEvent.Zero() {
		e := m.pendingEvent
		m.pendingEvent = Event{}
		m.SendEvent(e)
	}

}

// Return a string with a Graphviz DOT representation of the machine.
func (m *Machine) Export() string {
	export := `# dot -Tpng myfile.dot >myfile.png
digraph g {
  rankdir="LR";
  node[style="rounded",shape="box"]
  edge[splines="curved"]`
	export += "\n  " + m.currentState +
		" [style=\"rounded,filled\",fillcolor=\"gray\"]"
	for k, dst := range m.transitions {
		a := strings.SplitN(k, "_", 2)
		event, src := a[0], a[1]
		export += src + " -> " + dst + " [label=\"" + event + "\"];\n"
	}
	export += "}"
	return export
}

// Returns true if state is the current state
func (m *Machine) IsState(state string) bool {
	if m.currentState == state {
		return true
	}
	return false
}

// Returns true if event is a valid event from the current state
func (m *Machine) IsEvent(event string) bool {
	_, ok := m.transitions[event+"_"+m.currentState]
	if ok {
		return true
	}
	return false
}

// Rule adds a transition connecting an event (i.e., an arc or transition)
// between a pair of src and dst states.
func (m *Machine) Rule(event, src, dst string) {
	m.transitions[event+"_"+src] = dst
}

func (m *Machine) InState(src string) InState {
	return InState{src: src, m: m}
}

type InState struct {
	src string
	m   *Machine
}

func (x InState) On(evt string, dst string) Transition {
	x.m.Rule(evt, x.src, dst)
	return Transition{
		src: x.src,
		evt: evt,
		dst: dst,
		m:   x.m,
	}
}

type Transition struct {
	src string
	evt string
	dst string
	m   *Machine
}

func (x Transition) WithAction(fn func(event Event)) {
	x.m.actions[fmt.Sprintf("%s_%s_%s", x.evt, x.src, x.dst)] = fn
}
