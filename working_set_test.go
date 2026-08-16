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
	"database/sql"
	"fmt"
	"testing"

	"github.com/nustiueudinastea/dolt/go/libraries/doltcore/doltdb"
	"github.com/nustiueudinastea/dolt/go/libraries/doltcore/sqle/dsess"
	"github.com/nustiueudinastea/dolt/go/store/hash"
	"github.com/stretchr/testify/require"
)

func TestRefreshWorkingSetRootsUpdatesExistingSession(t *testing.T) {
	ctx := t.Context()
	db, conn := initializeTestDatabaseConnection(t, false)
	defer db.Close()
	defer conn.Close()

	_, err := conn.ExecContext(ctx, "create table items (id int primary key, value varchar(20))")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "insert into items values (1, 'one')")
	require.NoError(t, err)
	rootOne := requireWorkingRootHash(t, ctx, conn, "testdb", "main")

	_, err = conn.ExecContext(ctx, "insert into items values (2, 'two')")
	require.NoError(t, err)
	rootTwo := requireWorkingRootHash(t, ctx, conn, "testdb", "main")
	require.NotEqual(t, rootOne, rootTwo)
	requireResults(t, conn, "select count(*) from items", [][]any{{int64(2)}})

	err = conn.Raw(func(driverConn any) error {
		doltConn, ok := driverConn.(*DoltConn)
		if !ok {
			return fmt.Errorf("unexpected driver connection type %T", driverConn)
		}
		return doltConn.RefreshWorkingSetRoots(ctx, WorkingSetRootRefresh{
			Database:    "testdb",
			Branch:      "main",
			WorkingRoot: rootOne,
			StagedRoot:  rootOne,
		})
	})
	require.NoError(t, err)

	requireResults(t, conn, "select count(*) from items", [][]any{{int64(1)}})

	nextConn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer nextConn.Close()
	_, err = nextConn.ExecContext(ctx, "use testdb")
	require.NoError(t, err)
	requireResults(t, nextConn, "select count(*) from items", [][]any{{int64(1)}})
}

func requireWorkingRootHash(t *testing.T, ctx context.Context, conn *sql.Conn, database, branch string) hash.Hash {
	t.Helper()

	var root hash.Hash
	err := conn.Raw(func(driverConn any) error {
		doltConn, ok := driverConn.(*DoltConn)
		if !ok {
			return fmt.Errorf("unexpected driver connection type %T", driverConn)
		}
		sqlCtx := doltConn.gmsCtx.WithContext(ctx)
		ws, err := dsess.DSessFromSess(sqlCtx.Session).WorkingSet(sqlCtx, doltdb.RevisionDbName(database, branch))
		if err != nil {
			return err
		}
		root, err = ws.WorkingRoot().HashOf()
		return err
	})
	require.NoError(t, err)
	return root
}
