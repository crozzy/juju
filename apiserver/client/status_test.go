// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/charmrevisionupdater"
	"github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type statusSuite struct {
	baseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

// Complete testing of status functionality happens elsewhere in the codebase,
// these tests just sanity-check the api itself.

func (s *statusSuite) TestFullStatus(c *gc.C) {
	machine := s.addMachine(c)
	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Model.Name, gc.Equals, "controller")
	c.Check(status.Applications, gc.HasLen, 0)
	c.Check(status.Machines, gc.HasLen, 1)
	resultMachine, ok := status.Machines[machine.Id()]
	if !ok {
		c.Fatalf("Missing machine with id %q", machine.Id())
	}
	c.Check(resultMachine.Id, gc.Equals, machine.Id())
	c.Check(resultMachine.Series, gc.Equals, machine.Series())
}

var _ = gc.Suite(&statusUnitTestSuite{})

type statusUnitTestSuite struct {
	baseSuite
	*factory.Factory
}

func (s *statusUnitTestSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	// State gets reset per test, so must the factory.
	s.Factory = factory.NewFactory(s.State)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithOneMachineAndOneContainer(c *gc.C) {
	host := s.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("0")})
	container := s.MakeMachineNested(c, host.Id(), nil)
	machines := map[string][]*state.Machine{
		host.Id(): {host, container},
	}

	statuses := client.ProcessMachines(machines)
	c.Assert(statuses, gc.Not(gc.IsNil))

	containerStatus := client.MakeMachineStatus(container)
	c.Check(statuses[host.Id()].Containers[container.Id()].Id, gc.Equals, containerStatus.Id)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithEmbeddedContainers(c *gc.C) {
	host := s.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("1")})
	lxdHost := s.MakeMachineNested(c, host.Id(), nil)
	machines := map[string][]*state.Machine{
		host.Id(): {
			host,
			lxdHost,
			s.MakeMachineNested(c, lxdHost.Id(), nil),
			s.MakeMachineNested(c, host.Id(), nil),
		},
	}

	statuses := client.ProcessMachines(machines)
	c.Assert(statuses, gc.Not(gc.IsNil))

	hostContainer := statuses[host.Id()].Containers
	c.Check(hostContainer, gc.HasLen, 2)
	c.Check(hostContainer[lxdHost.Id()].Containers, gc.HasLen, 1)
}

var testUnits = []struct {
	unitName       string
	setStatus      *state.MeterStatus
	expectedStatus *params.MeterStatus
}{{
	setStatus:      &state.MeterStatus{Code: state.MeterGreen, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "green", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterAmber, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "amber", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterRed, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "red", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterGreen, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "green", Message: "test information"},
}, {},
}

func (s *statusUnitTestSuite) TestMeterStatus(c *gc.C) {
	service := s.MakeApplication(c, nil)

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := service.AddUnit()
		testUnits[i].unitName = u.Name()
		c.Assert(err, jc.ErrorIsNil)
		if unit.setStatus != nil {
			err := u.SetMeterStatus(unit.setStatus.Code.String(), unit.setStatus.Info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	serviceStatus, ok := status.Applications[service.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(serviceStatus.MeterStatuses, gc.HasLen, len(testUnits)-1)
	for _, unit := range testUnits {
		unitStatus, ok := serviceStatus.MeterStatuses[unit.unitName]

		if unit.expectedStatus != nil {
			c.Assert(ok, gc.Equals, true)
			c.Assert(&unitStatus, gc.DeepEquals, unit.expectedStatus)
		} else {
			c.Assert(ok, gc.Equals, false)
		}
	}
}

func (s *statusUnitTestSuite) checkWorkloadVersionAggregation(
	c *gc.C, expectedAppVersion string, unitVersions ...string) {
	application := s.MakeApplication(c, nil)

	units := make([]*state.Unit, len(unitVersions))
	for i, version := range unitVersions {
		unit, err := application.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		if version != "<unset>" {
			time.Sleep(time.Millisecond * 1)
			err = unit.SetWorkloadVersion(version)
			c.Assert(err, jc.ErrorIsNil)
		}
		units[i] = unit
	}

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	appStatus, found := status.Applications[application.Name()]
	c.Assert(found, jc.IsTrue)
	c.Check(appStatus.WorkloadVersion, gc.Equals, expectedAppVersion)

	for i, expectedVersion := range unitVersions {
		if expectedVersion == "<unset>" {
			expectedVersion = ""
		}
		unitStatus, found := appStatus.Units[units[i].Name()]
		c.Check(found, jc.IsTrue)
		c.Check(unitStatus.WorkloadVersion, gc.Equals, expectedVersion)
	}
}

func (s *statusUnitTestSuite) TestWorkloadVersionLastWins(c *gc.C) {
	s.checkWorkloadVersionAggregation(c, "zarkon",
		"voltron", "voltron", "zarkon")
}

func (s *statusUnitTestSuite) TestWorkloadVersionInTie(c *gc.C) {
	s.checkWorkloadVersionAggregation(c, "voltron",
		"allura", "zarkon", "voltron", "zarkon", "voltron")
}

func (s *statusUnitTestSuite) TestWorkloadVersionSimple(c *gc.C) {
	s.checkWorkloadVersionAggregation(c, "voltron", "voltron", "voltron")
}

func (s *statusUnitTestSuite) TestWorkloadVersionBlanksCanWin(c *gc.C) {
	s.checkWorkloadVersionAggregation(c, "", "voltron", "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionBlanksCanBeOverwritten(c *gc.C) {
	s.checkWorkloadVersionAggregation(c, "voltron", "voltron", "", "voltron")
}

func (s *statusUnitTestSuite) TestWorkloadVersionOnlyBlanks(c *gc.C) {
	s.checkWorkloadVersionAggregation(c, "", "", "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionOkWithUnset(c *gc.C) {
	s.checkWorkloadVersionAggregation(c, "", "<unset>", "<unset>")
}

type statusUpgradeUnitSuite struct {
	testing.CharmSuite
	jujutesting.JujuConnSuite

	charmrevisionupdater *charmrevisionupdater.CharmRevisionUpdaterAPI
	resources            *common.Resources
	authoriser           apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&statusUpgradeUnitSuite{})

func (s *statusUpgradeUnitSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.JujuConnSuite)
}

func (s *statusUpgradeUnitSuite) TearDownSuite(c *gc.C) {
	s.CharmSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *statusUpgradeUnitSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	var err error
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusUpgradeUnitSuite) TearDownTest(c *gc.C) {
	s.CharmSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *statusUpgradeUnitSuite) TestUpdateRevisions(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageModel)
	s.SetupScenario(c)
	client := s.APIState.Client()
	status, _ := client.Status(nil)

	serviceStatus, ok := status.Applications["mysql"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(serviceStatus.CanUpgradeTo, gc.Equals, "")

	// Update to the latest available charm revision.
	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// Check if CanUpgradeTo suggest the latest revision.
	status, _ = client.Status(nil)
	serviceStatus, ok = status.Applications["mysql"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(serviceStatus.CanUpgradeTo, gc.Equals, "cs:quantal/mysql-23")
}
