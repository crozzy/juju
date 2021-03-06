// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	apiapplication "github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
)

type getSuite struct {
	jujutesting.JujuConnSuite

	applicationAPI *application.API
	authorizer     apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&getSuite{})

func (s *getSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	backend, err := application.NewStateBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(s.State)
	s.applicationAPI, err = application.NewAPI(
		backend,
		s.authorizer,
		blockChecker,
		application.CharmToStateCharm,
		application.DeployApplication,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *getSuite) TestClientApplicationGetSmoketestV4(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	v4 := &application.APIv4{s.applicationAPI}
	results, err := v4.Get(params.ApplicationGet{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ApplicationGetResults{
		Application: "wordpress",
		Charm:       "wordpress",
		Config: map[string]interface{}{
			"blog-title": map[string]interface{}{
				"default":     true,
				"description": "A descriptive title used for the blog.",
				"type":        "string",
				"value":       "My Title",
			},
		},
		Series: "quantal",
	})
}

func (s *getSuite) TestClientApplicationGetSmoketest(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	results, err := s.applicationAPI.Get(params.ApplicationGet{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ApplicationGetResults{
		Application: "wordpress",
		Charm:       "wordpress",
		Config: map[string]interface{}{
			"blog-title": map[string]interface{}{
				"default":     "My Title",
				"description": "A descriptive title used for the blog.",
				"source":      "default",
				"type":        "string",
				"value":       "My Title",
			},
		},
		Series: "quantal",
	})
}

func (s *getSuite) TestApplicationGetUnknownApplication(c *gc.C) {
	_, err := s.applicationAPI.Get(params.ApplicationGet{"unknown"})
	c.Assert(err, gc.ErrorMatches, `application "unknown" not found`)
}

var getTests = []struct {
	about       string
	charm       string
	constraints string
	config      charm.Settings
	expect      params.ApplicationGetResults
}{{
	about:       "deployed application",
	charm:       "dummy",
	constraints: "mem=2G cpu-power=400",
	config: charm.Settings{
		// Different from default.
		"title": "Look To Windward",
		// Same as default.
		"username": "admin001",
		// Use default (but there's no charm default)
		"skill-level": nil,
		// Outlook is left unset.
	},
	expect: params.ApplicationGetResults{
		Config: map[string]interface{}{
			"title": map[string]interface{}{
				"default":     "My Title",
				"description": "A descriptive title used for the application.",
				"source":      "user",
				"type":        "string",
				"value":       "Look To Windward",
			},
			"outlook": map[string]interface{}{
				"description": "No default outlook.",
				"source":      "unset",
				"type":        "string",
			},
			"username": map[string]interface{}{
				"default":     "admin001",
				"description": "The name of the initial account (given admin permissions).",
				"source":      "default",
				"type":        "string",
				"value":       "admin001",
			},
			"skill-level": map[string]interface{}{
				"description": "A number indicating skill.",
				"source":      "unset",
				"type":        "int",
			},
		},
		Series: "quantal",
	},
}, {
	about: "deployed application  #2",
	charm: "dummy",
	config: charm.Settings{
		// Set title to default.
		"title": nil,
		// Value when there's a default.
		"username": "foobie",
		// Numeric value.
		"skill-level": 0,
		// String value.
		"outlook": "phlegmatic",
	},
	expect: params.ApplicationGetResults{
		Config: map[string]interface{}{
			"title": map[string]interface{}{
				"default":     "My Title",
				"description": "A descriptive title used for the application.",
				"source":      "default",
				"type":        "string",
				"value":       "My Title",
			},
			"outlook": map[string]interface{}{
				"description": "No default outlook.",
				"type":        "string",
				"source":      "user",
				"value":       "phlegmatic",
			},
			"username": map[string]interface{}{
				"default":     "admin001",
				"description": "The name of the initial account (given admin permissions).",
				"source":      "user",
				"type":        "string",
				"value":       "foobie",
			},
			"skill-level": map[string]interface{}{
				"description": "A number indicating skill.",
				"source":      "user",
				"type":        "int",
				// TODO(jam): 2013-08-28 bug #1217742
				// we have to use float64() here, because the
				// API does not preserve int types. This used
				// to be int64() but we end up with a type
				// mismatch when comparing the content
				"value": float64(0),
			},
		},
		Series: "quantal",
	},
}, {
	about: "subordinate application",
	charm: "logging",
	expect: params.ApplicationGetResults{
		Config: map[string]interface{}{},
		Series: "quantal",
	},
}}

func (s *getSuite) TestApplicationGet(c *gc.C) {
	for i, t := range getTests {
		c.Logf("test %d. %s", i, t.about)
		ch := s.AddTestingCharm(c, t.charm)
		app := s.AddTestingApplication(c, fmt.Sprintf("test%d", i), ch)

		var constraintsv constraints.Value
		if t.constraints != "" {
			constraintsv = constraints.MustParse(t.constraints)
			err := app.SetConstraints(constraintsv)
			c.Assert(err, jc.ErrorIsNil)
		}
		if t.config != nil {
			err := app.UpdateCharmConfig(t.config)
			c.Assert(err, jc.ErrorIsNil)
		}
		expect := t.expect
		expect.Constraints = constraintsv
		expect.Application = app.Name()
		expect.Charm = ch.Meta().Name
		client := apiapplication.NewClient(s.APIState)
		got, err := client.Get(app.Name())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*got, jc.DeepEquals, expect)
	}
}

func (s *getSuite) TestGetMaxResolutionInt(c *gc.C) {
	// See the bug http://pad.lv/1217742
	// Get ends up pushing a map[string]interface{} which containts
	// an int64 through a JSON Marshal & Unmarshal which ends up changing
	// the int64 into a float64. We will fix it if we find it is actually a
	// problem.
	const nonFloatInt = (int64(1) << 54) + 1
	const asFloat = float64(nonFloatInt)
	c.Assert(int64(asFloat), gc.Not(gc.Equals), nonFloatInt)
	c.Assert(int64(asFloat)+1, gc.Equals, nonFloatInt)

	ch := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "test-application", ch)

	err := app.UpdateCharmConfig(map[string]interface{}{"skill-level": nonFloatInt})
	c.Assert(err, jc.ErrorIsNil)
	client := apiapplication.NewClient(s.APIState)
	got, err := client.Get(app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.Config["skill-level"], jc.DeepEquals, map[string]interface{}{
		"description": "A number indicating skill.",
		"source":      "user",
		"type":        "int",
		"value":       asFloat,
	})
}
