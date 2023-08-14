/*
Copyright The CloudNativePG Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tablespaces

import (
	"context"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/management/controller/tablespaces/infrastructure"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/log"
)

var reservedTablespaceName = map[string]interface{}{
	"pg_default": nil,
	"pg_global":  nil,
}

type (
	// TablespaceAction encodes the action necessary for a tablespaceAction
	TablespaceAction string
	// TablespaceByAction tablespaces group by action need to take
	TablespaceByAction     map[TablespaceAction][]TablespaceConfigurationAdapter
	tablespaceNameByStatus map[apiv1.TablespaceStatus][]string
)

// possible role actions
const (
	// TbsIsReconciled tablespaces action represent tablespace already reconciled
	TbsIsReconciled TablespaceAction = "RECONCILED"
	// TbsToCreate tablespaces action represent tablespace going to create
	TbsToCreate TablespaceAction = "CREATE"
	// TbsToUpdate tablespaces action represent tablespace going to update
	TbsToUpdate TablespaceAction = "UPDATE"
	// TbsReserved tablespaces which is reserved by operator
	TbsReserved TablespaceAction = "RESERVED"
)

// TablespaceConfigurationAdapter the adapter class for tablespace configuration
type TablespaceConfigurationAdapter struct {
	// Name tablespace name
	Name string
	// TablespaceConfiguration tablespace with configuration settings
	apiv1.TablespaceConfiguration
}

// IsTablespaceNameReserved checks if a tablespace is reserved for PostgreSQL
// or the operator
func IsTablespaceNameReserved(name string) bool {
	if _, isReserved := reservedTablespaceName[name]; isReserved {
		return isReserved
	}
	return false
}

// convertToTablespaceNameByStatus gets the tablespace Name of every status
func (r TablespaceByAction) convertToTablespaceNameByStatus() tablespaceNameByStatus {
	statusByAction := map[TablespaceAction]apiv1.TablespaceStatus{
		TbsIsReconciled: apiv1.TablespaceStatusReconciled,
		TbsToCreate:     apiv1.TablespaceStatusPendingReconciliation,
		TbsToUpdate:     apiv1.TablespaceStatusPendingReconciliation,
		TbsReserved:     apiv1.TablespaceStatusReserved,
	}

	tablespaceByStatus := make(tablespaceNameByStatus)
	for action, tbsAdapterSlice := range r {
		for _, tbsAdapter := range tbsAdapterSlice {
			tablespaceByStatus[statusByAction[action]] = append(tablespaceByStatus[statusByAction[action]],
				tbsAdapter.Name)
		}
	}

	return tablespaceByStatus
}

// EvaluateNextActions evaluates the action needed for each role in the DB and/or the Spec.
// It has no side effects
func EvaluateNextActions(
	ctx context.Context,
	tablespaceInDBSlice []infrastructure.Tablespace,
	tablespaceInSpecMap map[string]*apiv1.TablespaceConfiguration,
) TablespaceByAction {
	contextLog := log.FromContext(ctx).WithName("tablespace_reconciler")
	contextLog.Debug("evaluating tablespace actions")

	tablespaceByAction := make(TablespaceByAction)

	tbsInDBNamed := make(map[string]infrastructure.Tablespace)
	for idx, tbs := range tablespaceInDBSlice {
		tbsInDBNamed[tbs.Name] = tablespaceInDBSlice[idx]
	}
	for tbsInSpecName, tbsInSpec := range tablespaceInSpecMap {
		tbsInDB, isTbsInDB := tbsInDBNamed[tbsInSpecName]
		switch {
		case IsTablespaceNameReserved(tbsInSpecName):
			tablespaceByAction[TbsReserved] = append(tablespaceByAction[TbsReserved],
				tablespaceAdapterFromName(tbsInSpecName, *tbsInSpec))
		case isTbsInDB && tbsInSpec.Temporary != tbsInDB.Temporary:
			tablespaceByAction[TbsToUpdate] = append(tablespaceByAction[TbsToUpdate],
				tablespaceAdapterFromName(tbsInSpecName, *tbsInSpec))
		case !isTbsInDB:
			tablespaceByAction[TbsToCreate] = append(tablespaceByAction[TbsToCreate],
				tablespaceAdapterFromName(tbsInSpecName, *tbsInSpec))
		default:
			tablespaceByAction[TbsIsReconciled] = append(tablespaceByAction[TbsIsReconciled],
				tablespaceAdapterFromName(tbsInSpecName, *tbsInSpec))
		}
	}

	return tablespaceByAction
}

func tablespaceAdapterFromName(tbsName string, tbsConfig apiv1.TablespaceConfiguration) TablespaceConfigurationAdapter {
	return TablespaceConfigurationAdapter{Name: tbsName, TablespaceConfiguration: tbsConfig}
}
