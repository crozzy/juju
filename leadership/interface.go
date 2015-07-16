// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/lease"
)

// ErrClaimDenied is the error which will be returned when a
// leadership claim has been denied.
var ErrClaimDenied = errors.New("leadership claim denied")

// Claimer exposes leadership acquisition capabilities.
type Claimer interface {

	// ClaimLeadership claims leadership of the named service on behalf of the
	// named unit. If no error is returned, leadership will be guaranteed for
	// at least the supplied duration from the point when the call was made.
	ClaimLeadership(serviceId, unitId string, duration time.Duration) error

	// BlockUntilLeadershipReleased blocks until the named service is known
	// to have no leader, in which case it returns no error; or until the
	// manager is stopped, in which case it will fail.
	BlockUntilLeadershipReleased(serviceId string) (err error)
}

// Token represents a unit's leadership of its service.
type Token interface {

	// Check returns an error if the condition it embodies no longer holds.
	// If you pass a non-nil value into Check, it must be a pointer to data
	// of the correct type (which you are expected to know by magic), into
	// which the token's content will be copied.
	//
	// In practice, most implementations will likely expect *[]txn.Op, so
	// that they can be used to gate mgo/txn-based state changes.
	Check(interface{}) error
}

// Checker exposes leadership testing capabilities.
type Checker interface {

	// LeadershipCheck verifies that the named unit is leader of the named
	// service, and returns a Token attesting to that fact for use building
	// mgo/txn transactions that depend upon it.
	LeadershipCheck(serviceName, unitName string) Token
}

type LeadershipLeaseManager interface {

	// Claimlease claims a lease for the given duration for the given
	// namespace and id. If the lease is already owned, a
	// LeaseClaimDeniedErr will be returned. Either way the current lease
	// owner's ID will be returned.
	ClaimLease(namespace, id string, forDur time.Duration) (leaseOwnerId string, err error)

	// ReleaseLease releases the lease held for namespace by id.
	ReleaseLease(namespace, id string) (err error)

	// RetrieveLease retrieves the current lease token for a given
	// namespace. This is not intended to be exposed to clients, and is
	// only available within a server-process.
	RetrieveLease(namespace string) (lease.Token, error)

	// LeaseReleasedNotifier returns a channel a caller can block on to be
	// notified of when a lease is released for namespace. The caller must
	// use a comma-ok to decide whether or not the channel was notified or
	// was closed: if notified, then the lease has been released; if closed,
	// then the lease manager has been disrupted before it could notify
	// the caller.
	LeaseReleasedNotifier(namespace string) (notifier <-chan struct{}, err error)
}
