// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// AddressAndCertGetter can be used to find out
// controller addresses and the CA public certificate.
type AddressAndCertGetter interface {
	Addresses() ([]string, error)
	ModelUUID() string
	APIHostPorts() ([][]network.HostPort, error)
	WatchAPIHostPorts() state.NotifyWatcher
}

// APIAddresser implements the APIAddresses method
type APIAddresser struct {
	resources facade.Resources
	getter    AddressAndCertGetter
}

// NewAPIAddresser returns a new APIAddresser that uses the given getter to
// fetch its addresses.
func NewAPIAddresser(getter AddressAndCertGetter, resources facade.Resources) *APIAddresser {
	return &APIAddresser{
		getter:    getter,
		resources: resources,
	}
}

// APIHostPorts returns the API server addresses.
func (api *APIAddresser) APIHostPorts() (params.APIHostPortsResult, error) {
	servers, err := api.getter.APIHostPorts()
	if err != nil {
		return params.APIHostPortsResult{}, err
	}
	return params.APIHostPortsResult{
		Servers: params.FromNetworkHostsPorts(servers),
	}, nil
}

// WatchAPIHostPorts watches the API server addresses.
func (api *APIAddresser) WatchAPIHostPorts() (params.NotifyWatchResult, error) {
	watch := api.getter.WatchAPIHostPorts()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{}, watcher.EnsureErr(watch)
}

// APIAddresses returns the list of addresses used to connect to the API.
func (api *APIAddresser) APIAddresses() (params.StringsResult, error) {
	addrs, err := apiAddresses(api.getter)
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}

func apiAddresses(getter APIHostPortsGetter) ([]string, error) {
	apiHostPorts, err := getter.APIHostPorts()
	if err != nil {
		return nil, err
	}
	var addrs = make([]string, 0, len(apiHostPorts))
	for _, hostPorts := range apiHostPorts {
		ordered := network.PrioritizeInternalHostPorts(hostPorts, false)
		for _, addr := range ordered {
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}
	return addrs, nil
}

// ModelUUID returns the model UUID to connect to the environment
// that the current connection is for.
func (a *APIAddresser) ModelUUID() params.StringResult {
	return params.StringResult{Result: a.getter.ModelUUID()}
}

// StateAddresser implements a common set of methods for getting state
// server addresses, and the CA certificate used to authenticate them.
type StateAddresser struct {
	getter AddressAndCertGetter
}

// NewStateAddresser returns a new StateAddresser that uses the given
// st value to fetch its addresses.
func NewStateAddresser(getter AddressAndCertGetter) *StateAddresser {
	return &StateAddresser{getter}
}

// StateAddresses returns the list of addresses used to connect to the state.
func (a *StateAddresser) StateAddresses() (params.StringsResult, error) {
	addrs, err := a.getter.Addresses()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}
