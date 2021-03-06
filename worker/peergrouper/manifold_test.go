// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold     dependency.Manifold
	context      dependency.Context
	clock        *testing.Clock
	agent        *mockAgent
	hub          *mockHub
	st           *state.State
	pool         *state.StatePool
	stateTracker stubStateTracker

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.clock = testing.NewClock(time.Time{})
	s.agent = &mockAgent{conf: mockAgentConfig{
		info: &params.StateServingInfo{
			StatePort: 1234,
			APIPort:   5678,
		},
	}}
	s.hub = &mockHub{}
	s.st = new(state.State)
	s.pool = state.NewStatePool(s.st)
	s.stateTracker = stubStateTracker{pool: s.pool}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = peergrouper.Manifold(peergrouper.ManifoldConfig{
		AgentName: "agent",
		ClockName: "clock",
		StateName: "state",
		Hub:       s.hub,
		NewWorker: s.newWorker,
		ControllerSupportsSpaces: func(st *state.State) (bool, error) {
			if st != s.st {
				return false, errors.New("invalid state")
			}
			return true, nil
		},
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"clock": s.clock,
		"state": &s.stateTracker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config peergrouper.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
	return w, nil
}

var expectedInputs = []string{"agent", "clock", "state"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, peergrouper.Config{})
	config := args[0].(peergrouper.Config)

	c.Assert(config, jc.DeepEquals, peergrouper.Config{
		State:        peergrouper.StateShim{s.st},
		MongoSession: peergrouper.MongoSessionShim{},
		APIHostPortsSetter: &peergrouper.CachingAPIHostPortsSetter{
			APIHostPortsSetter: s.st,
		},
		Clock:          s.clock,
		Hub:            s.hub,
		SupportsSpaces: true,
		MongoPort:      1234,
		APIPort:        5678,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type stubStateTracker struct {
	testing.Stub
	pool *state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockAgentConfig struct {
	agent.Config
	info *params.StateServingInfo
}

func (c *mockAgentConfig) StateServingInfo() (params.StateServingInfo, bool) {
	if c.info != nil {
		return *c.info, true
	}
	return params.StateServingInfo{}, false
}

type mockHub struct {
	peergrouper.Hub
}
