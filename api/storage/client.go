// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"strings"
)

var logger = loggo.GetLogger("juju.api.storage")

// Client allows access to the storage API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the storage API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Storage")
	logger.Debugf("\nSTORAGE FRONT-END: %#v", frontend)
	logger.Debugf("\nSTORAGE BACK-END: %#v", backend)
	return &Client{ClientFacade: frontend, facade: backend}
}

func (c *Client) Show(tags []names.StorageTag) ([]params.StorageInstance, error) {
	found := params.StorageShowResults{}
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i] = params.Entity{Tag: tag.String()}
	}
	if err := c.facade.FacadeCall("Show", params.Entities{Entities: entities}, &found); err != nil {
		return nil, errors.Trace(err)
	}
	info := []params.StorageInstance{}
	var errorStrings []string
	for _, result := range found.Results {
		if result.Error != nil {
			errorStrings = append(errorStrings, result.Error.Error())
			continue
		}
		info = append(info, result.Result)
	}
	if len(errorStrings) > 0 {
		return nil, errors.New(strings.Join(errorStrings, ", "))
	}
	return info, nil
}
