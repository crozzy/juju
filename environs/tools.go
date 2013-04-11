package environs

import (
	"fmt"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

// ToolsList holds a list of available tools.  Private tools take
// precedence over public tools, even if they have a lower
// version number.
type ToolsList struct {
	Private tools.List
	Public  tools.List
}

// ListTools returns a ToolsList holding all the tools
// available in the given environment that have the
// given major version.
func ListTools(env Environ, majorVersion int) (*ToolsList, error) {
	private, err := tools.ReadList(env.Storage(), majorVersion)
	if err != nil && err != tools.ErrNoMatches {
		return nil, err
	}
	public, err := tools.ReadList(env.PublicStorage(), majorVersion)
	if err != nil && err != tools.ErrNoMatches {
		return nil, err
	}
	return &ToolsList{
		Private: private,
		Public:  public,
	}, nil
}

// BestTools returns the most recent version
// from the set of tools in the ToolsList that are
// compatible with the given version, using flags
// to determine possible candidates.
// It returns nil if no such tools are found.
func BestTools(list *ToolsList, vers version.Binary, flags ToolsSearchFlags) *state.Tools {
	if flags&CompatVersion == 0 {
		panic("CompatVersion not implemented")
	}
	if tools := bestTools(list.Private, vers, flags); tools != nil {
		return tools
	}
	return bestTools(list.Public, vers, flags)
}

// bestTools is like BestTools but operates on a single list of tools.
func bestTools(toolsList []*state.Tools, vers version.Binary, flags ToolsSearchFlags) *state.Tools {
	var bestTools *state.Tools
	allowDev := vers.IsDev() || flags&DevVersion != 0
	allowHigher := flags&HighestVersion != 0
	log.Debugf("finding best tools for version: %v", vers)
	for _, t := range toolsList {
		log.Debugf("checking tools %v", t)
		if t.Major != vers.Major ||
			t.Series != vers.Series ||
			t.Arch != vers.Arch ||
			!allowDev && t.IsDev() ||
			!allowHigher && vers.Number.Less(t.Number) {
			continue
		}
		if bestTools == nil || bestTools.Number.Less(t.Number) {
			bestTools = t
		}
	}
	return bestTools
}

// ToolsSearchFlags gives options when searching
// for tools.
type ToolsSearchFlags int

const (
	// HighestVersion indicates that versions above the version being
	// searched for may be included in the search. The default behavior
	// is to search for versions <= the one provided.
	HighestVersion ToolsSearchFlags = 1 << iota

	// DevVersion includes development versions in the search, even
	// when the version to match against isn't a development version.
	DevVersion

	// CompatVersion specifies that the major version number
	// must be the same as specified. At the moment this flag is required.
	CompatVersion
)

// FindTools tries to find a set of tools compatible with the given
// version from the given environment, using flags to determine
// possible candidates.
//
// If no tools are found and there's no other error, a NotFoundError is
// returned.  If there's anything compatible in the environ's Storage,
// it gets precedence over anything in its PublicStorage.
func FindTools(env Environ, vers version.Binary, flags ToolsSearchFlags) (*state.Tools, error) {
	log.Infof("environs: searching for tools compatible with version: %v\n", vers)
	toolsList, err := ListTools(env, vers.Major)
	if err != nil {
		return nil, err
	}
	tools := BestTools(toolsList, vers, flags)
	if tools == nil {
		return tools, &NotFoundError{fmt.Errorf("no compatible tools found")}
	}
	return tools, nil
}

// FindAvailableTools returns a tools.List containing all tools with a given
// version number in the environment's private storage. If no tools are
// present in private storage, it falls back to public storage; if no tools
// are present there, it returns ErrNoTools. Tools from public and private
// buckets are not mixed.
func FindAvailableTools(environ Environ, majorVersion int) (tools.List, error) {
	list, err := tools.ReadList(environ.Storage(), majorVersion)
	if err == tools.ErrNoMatches {
		list, err = tools.ReadList(environ.PublicStorage(), majorVersion)
	}
	return list, err
}

// FindBootstrapTools returns a ToolsList containing only those tools with
// which it would be reasonable to launch an environment's first machine,
// given the supplied constraints.
// If the environment was not already configured to use a specific agent
// version, the newest available version will be chosen and set in the
// environment's configuration.
func FindBootstrapTools(environ Environ, cons constraints.Value) (list tools.List, err error) {
	defer noMatchContext(&err)
	// Collect all possible compatible tools.
	cliVersion := version.CurrentNumber()
	if list, err = FindAvailableTools(environ, cliVersion.Major); err != nil {
		return nil, err
	}

	// Discard all that are known to be irrelevant.
	cfg := environ.Config()
	filter := tools.Filter{Series: cfg.DefaultSeries()}
	if cons.Arch != nil && *cons.Arch != "" {
		filter.Arch = *cons.Arch
	}
	if agentVersion, ok := cfg.AgentVersion(); ok {
		// If we already have an explicit agent version set, we're done.
		filter.Number = agentVersion
		return list.Match(filter)
	}
	allowDevTools := cfg.Development() || cliVersion.IsDev()
	filter.Released = !allowDevTools
	if list, err = list.Match(filter); err != nil {
		return nil, err
	}

	// We probably still have a mix of versions available; discard older ones
	// and update environment configuration to use only those remaining.
	list = list.Newest()
	if cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": list[0].Number.String(),
	}); err == nil {
		err = environ.SetConfig(cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update environment configuration: %v", err)
	}
	return list, nil
}

// FindInstanceTools returns a ToolsList containing only those tools with which
// it would be reasonable to start a new instance, given the supplied series and
// constraints.
// It is an error to call it with an environment not already configured to use
// a specific agent version.
func FindInstanceTools(environ Environ, series string, cons constraints.Value) (list tools.List, err error) {
	defer noMatchContext(&err)
	// Collect all possible compatible tools.
	agentVersion, ok := environ.Config().AgentVersion()
	if !ok {
		return nil, fmt.Errorf("no agent version set in environment configuration")
	}
	if list, err = FindAvailableTools(environ, agentVersion.Major); err != nil {
		return nil, err
	}

	// Discard all that are known to be irrelevant.
	filter := tools.Filter{
		Number: agentVersion,
		Series: series,
	}
	if cons.Arch != nil && *cons.Arch != "" {
		filter.Arch = *cons.Arch
	}
	return list.Match(filter)
}

// noMatchContext converts tools.ErrNoMatches into a NotFoundError.
func noMatchContext(err *error) {
	if *err == tools.ErrNoMatches {
		*err = &NotFoundError{*err}
	}
}

// FindExactTools returns only the tools that match the supplied version.
func FindExactTools(environ Environ, vers version.Binary) (*state.Tools, error) {
	list, err := FindAvailableTools(environ, vers.Major)
	if err != nil {
		return nil, err
	}
	if list, err = list.Match(tools.Filter{
		Number: vers.Number,
		Series: vers.Series,
		Arch:   vers.Arch,
	}); err != nil {
		return nil, err
	}
	return list[0], nil
}
