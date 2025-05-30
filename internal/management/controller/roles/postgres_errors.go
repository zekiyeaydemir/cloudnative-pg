/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package roles

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// RoleError is an EXPECTABLE error when performing role-related actions on the
// database. For example, we might try to drop a role that owns objects.
//
// RoleError is NOT meant to represent unexpected errors such as a panic or a
// connection interruption
type RoleError struct {
	RoleName string
	Cause    string
	Action   string
}

// Error returns a description for the error,
// … and lets RoleError comply with the `error` interface
func (re RoleError) Error() string {
	return fmt.Sprintf("could not perform %s on role %s: %s",
		re.Action, re.RoleName, re.Cause)
}

// parseRoleError matches an error to one of the expectable RoleError's
// If it matches a known case it returns a RoleError object otherwise it returns an error.
//
// For PostgreSQL codes see https://www.postgresql.org/docs/current/errcodes-appendix.html
func parseRoleError(err error, roleName string, action roleAction) (*RoleError, error) {
	var errPGX *pgconn.PgError
	if !errors.As(err, &errPGX) {
		return nil, fmt.Errorf("while trying to %s: %w", action, err)
	}

	knownCauses := map[string]string{
		"2BP01": errPGX.Detail,  // 2BP01 -> dependent_objects_still_exist
		"42704": errPGX.Message, // 42704 -> undefined_object
		"0LP01": errPGX.Message, // 0LP01 -> invalid_grant_operation
	}

	if cause, known := knownCauses[errPGX.Code]; known {
		return &RoleError{
			Action:   string(action),
			RoleName: roleName,
			Cause:    cause,
		}, nil
	}
	return nil, fmt.Errorf("while trying to %s: %w", action, err)
}
