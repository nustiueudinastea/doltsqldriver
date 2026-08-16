// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package embedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nustiueudinastea/dolt/go/libraries/doltcore/doltdb"
	"github.com/nustiueudinastea/dolt/go/libraries/doltcore/ref"
	"github.com/nustiueudinastea/dolt/go/libraries/doltcore/sqle/dsess"
	"github.com/nustiueudinastea/dolt/go/store/hash"
)

// WorkingSetRootRefresh describes a branch working-set update that should be
// made visible to the embedded SQL session that owns the connection.
type WorkingSetRootRefresh struct {
	// Database is the base database name, or a revision-qualified database name.
	// If empty, the connection's current database is used.
	Database string

	// Branch is the branch whose working set should be refreshed. If empty,
	// Database must be revision-qualified and its revision is used.
	Branch string

	WorkingRoot hash.Hash
	StagedRoot  hash.Hash
}

// RefreshWorkingSetRoots updates the persisted branch working set and refreshes
// this connection's in-memory Dolt SQL session so subsequent statements observe
// the supplied roots without closing and reopening the embedded engine.
func (d *DoltConn) RefreshWorkingSetRoots(ctx context.Context, refresh WorkingSetRootRefresh) error {
	if d == nil {
		return fmt.Errorf("dolt connection is nil")
	}
	if d.gmsCtx == nil {
		return fmt.Errorf("dolt connection has no SQL context")
	}
	if d.activeQueryCtx != nil {
		return fmt.Errorf("cannot refresh working set while a query is active")
	}
	if refresh.WorkingRoot.IsEmpty() {
		return fmt.Errorf("working root is empty")
	}
	if refresh.StagedRoot.IsEmpty() {
		return fmt.Errorf("staged root is empty")
	}

	baseName, branch, err := d.resolveRefreshTarget(refresh)
	if err != nil {
		return err
	}

	sessionDB := doltdb.RevisionDbName(baseName, branch)
	sqlCtx := d.gmsCtx.WithContext(ctx)
	doltSession := dsess.DSessFromSess(sqlCtx.Session)
	sqlDB, ok, err := doltSession.Provider().SessionDatabase(sqlCtx, sessionDB)
	if err != nil {
		return fmt.Errorf("load database %s: %w", sessionDB, err)
	}
	if !ok {
		return fmt.Errorf("database %s not found", sessionDB)
	}
	ddb := sqlDB.DbData().Ddb
	if ddb == nil {
		return fmt.Errorf("database %s has no DoltDB", sessionDB)
	}

	workingRoot, err := ddb.ReadRootValue(ctx, refresh.WorkingRoot)
	if err != nil {
		return fmt.Errorf("read working root %s: %w", refresh.WorkingRoot, err)
	}
	stagedRoot, err := ddb.ReadRootValue(ctx, refresh.StagedRoot)
	if err != nil {
		return fmt.Errorf("read staged root %s: %w", refresh.StagedRoot, err)
	}

	wsRef, err := ref.WorkingSetRefForHead(ref.NewBranchRef(branch))
	if err != nil {
		return fmt.Errorf("resolve working set ref for branch %q: %w", branch, err)
	}
	ws, err := ddb.ResolveWorkingSet(ctx, wsRef)
	var currentHash hash.Hash
	if errors.Is(err, doltdb.ErrWorkingSetNotFound) {
		ws = doltdb.EmptyWorkingSet(wsRef)
	} else if err != nil {
		return fmt.Errorf("resolve working set %s: %w", wsRef.String(), err)
	} else {
		currentHash, err = ws.HashOf()
		if err != nil {
			return fmt.Errorf("hash working set %s: %w", wsRef.String(), err)
		}
	}

	next := ws.WithWorkingRoot(workingRoot).
		WithStagedRoot(stagedRoot).
		ClearMerge().
		ClearRebase()
	if err := ddb.UpdateWorkingSet(ctx, wsRef, next, currentHash, doltdb.TodoWorkingSetMeta(), nil); err != nil {
		return fmt.Errorf("update working set %s: %w", wsRef.String(), err)
	}

	if err := doltSession.RemoveDbState(sqlCtx, baseName); err != nil {
		return fmt.Errorf("invalidate session state %s: %w", baseName, err)
	}
	if _, err := doltSession.WorkingSet(sqlCtx, sessionDB); err != nil {
		return fmt.Errorf("reload session working set %s: %w", sessionDB, err)
	}

	return nil
}

func (d *DoltConn) resolveRefreshTarget(refresh WorkingSetRootRefresh) (string, string, error) {
	database := strings.TrimSpace(refresh.Database)
	if database == "" {
		database = strings.TrimSpace(d.gmsCtx.GetCurrentDatabase())
	}
	baseName, revision := doltdb.SplitRevisionDbName(database)
	baseName = strings.TrimSpace(baseName)
	branch := strings.TrimSpace(refresh.Branch)
	if branch == "" {
		branch = strings.TrimSpace(revision)
	}
	if baseName == "" {
		return "", "", fmt.Errorf("database is required")
	}
	if branch == "" {
		return "", "", fmt.Errorf("branch is required")
	}
	return baseName, branch, nil
}
