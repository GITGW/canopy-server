/*
 * Copyright 2014 Gregory Prisament
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package cassandra_datalayer

//
// Cassandra stores data in column families (aka tables).  Each column family
// (table) has multiple rows.  Each row has a row key.  Each row also has an
// internal table of key-value pairs (aka "internal rows" or "cells").
// 
//  COLUMN FAMILY
//
//      +--------------------------------+
//      | [ROW_KEY0]                     |
//      |      KEY0: VALUE0              |
//      |      KEY1: VALUE1              |
//      |      ...                       |
//      +--------------------------------+
//      | [ROW_KEY1]                     |
//      |      KEY0: VALUE0              |
//      |      KEY1: VALUE1              |
//      |      ...                       |
//      +--------------------------------+
//      |                                |
//
// The internal keys are stored in sorted order within a row.  A row's contents
// is never split across nodes.
//
// For storing simple data, we could use:
//
//      CREATE TABLE propval_<datatype> (
//          device_id uuid,
//          propname text,
//          time timestamp,
//          value <datatype>,
//          PRIMARY KEY (device_id, propname, time)
//      ) WITH COMPACT STORAGE
//
//  Which maps to (for example):
//
//      propval_int
//
//      +---------------------------------+
//      | device_id (row key)             |
//      |      propname|timestamp : value |
//      |      propname|timestamp : value |
//      |      propname|timestamp : value |
//      +---------------------------------+
//      |                                 |
//
// Instead we use:
//
//      CREATE TABLE propval_<datatype> (
//          device_id uuid,
//          propname text,
//          time timestamp,
//          value <datatype>,
//          PRIMARY KEY ((device_id, propname), time)
//      ) WITH COMPACT STORAGE
//
//  Which maps to (for example):
//
//      propval_int
//
//      +---------------------------------+
//      | device_id|propname (row key)    |
//      |      timestamp : value          |
//      |      timestamp : value          |
//      |      timestamp : value          |
//      +---------------------------------+
//      |                                 |
//
//      Note that the concatenation of property name and timestamp is used as
//      the internal keys.
//
//
//  In theory we could put this all in a single column family (rather than
//  having a separate one for each datatype).  However, CQL does not appear to
//  have the flexibility to do this efficiently.  If we tried, for example:
//
//      CREATE TABLE propval_<datatype> (
//          device_id uuid,
//          propname text,
//          time timestamp,
//          value_int int,
//          value_bigint bigint,
//          value_string text,
//          PRIMARY KEY (device_id, propname, time)
//      ) WITH COMPACT STORAGE
//
//
//  The result would be:
//
//      +------------------------------------------------+
//      | device_id (row key)                            |
//      |      propname|timestamp|"value_int" : value    |
//      |      propname|timestamp|"value_int" : value    |
//      |      propname|timestamp|"value_int" : value    |
//      |                                                |
//      |      propname|timestamp|"value_string" : value |
//      |      propname|timestamp|"value_string" : value |
//      +------------------------------------------------+
//      |                                                |
//
//  Which is not nearly as efficient, because it would store, literaly, the
//  word "value_int" alongside each 32-bit integer data sample.
//
//  So instead, we create a separate table for each datatype.
//
//
//  You can gain insight into the actual structure of a CF by running:
//
//      > cassandra-cli
//      > use canopy;
//      > list propval_float;
//
//  Also useful:
//      > nodetool cfstats
//

/* Very useful: http://www.datastax.com/dev/blog/thrift-to-cql3 */
import (
    "github.com/gocql/gocql"
    "log"
)
var creationQueries []string = []string{
    // used for:
    //  uint8
    //  int8
    //  int16
    //  uint16
    //  int32
    //  uint32
    `CREATE TABLE propval_int (
        device_id uuid,
        propname text,
        time timestamp,
        value int,
        PRIMARY KEY((device_id, propname), time)
    ) WITH COMPACT STORAGE`,

    // used for:
    //  float32
    `CREATE TABLE propval_float (
        device_id uuid,
        propname text,
        time timestamp,
        value float,
        PRIMARY KEY((device_id, propname), time)
    ) WITH COMPACT STORAGE`,

    // used for:
    //  float64
    `CREATE TABLE propval_double (
        device_id uuid,
        propname text,
        time timestamp,
        value double,
        PRIMARY KEY((device_id, propname), time)
    ) WITH COMPACT STORAGE`,

    // used for:
    //  datetime
    `CREATE TABLE propval_timestamp (
        device_id uuid,
        propname text,
        time timestamp,
        value timestamp,
        PRIMARY KEY((device_id, propname), time)
    ) WITH COMPACT STORAGE`,

    // used for:
    //  bool
    `CREATE TABLE propval_boolean (
        device_id uuid,
        propname text,
        time timestamp,
        value boolean,
        PRIMARY KEY((device_id, propname), time)
    ) WITH COMPACT STORAGE`,

    // used for:
    //  void
    `CREATE TABLE propval_void (
        device_id uuid,
        propname text,
        time timestamp,
        PRIMARY KEY((device_id, propname), time)
    ) WITH COMPACT STORAGE`,

    // used for:
    //  string
    `CREATE TABLE propval_string (
        device_id uuid,
        propname text,
        time timestamp,
        value text
        PRIMARY KEY((device_id, propname), time)
    ) WITH COMPACT STORAGE`,


    `CREATE TABLE sensor_data (
        device_id uuid,
        propname text,
        time timestamp,
        value double,
        PRIMARY KEY(device_id, propname, time)
    ) WITH COMPACT STORAGE`,

    `CREATE TABLE devices (
        device_id uuid,
        friendly_name text,
        sddl text,
        PRIMARY KEY(device_id)
    ) WITH COMPACT STORAGE`,

    `CREATE TABLE device_group (
        username text,
        group_name text,
        group_order int,
        device_id uuid,
        device_friendly_name text,
        PRIMARY KEY(username, group_name, group_order)
    )`,

    `CREATE TABLE control_event (
        device_id uuid,
        time_issued timestamp,
        control_name text,
        value double,
        PRIMARY KEY(device_id, time_issued)
    )`,

    `CREATE TABLE device_permissions (
        username text,
        device_id uuid,
        access_level int,
        PRIMARY KEY(username, device_id)
    ) WITH COMPACT STORAGE`,

    `CREATE TABLE accounts (
        username text,
        email text,
        password_hash blob,
        PRIMARY KEY(username)
    ) WITH COMPACT STORAGE`,

    `CREATE TABLE account_emails (
        email text,
        username text,
        PRIMARY KEY(email)
    ) WITH COMPACT STORAGE`,
}

type CassDatalayer struct {
}

func NewCassDatalayer() *CassDatalayer {
    return &CassDatalayer{}
}

func (dl *CassDatalayer) Connect(keyspace string) Connection, error {
    cluster := gocql.NewCluster("127.0.0.1")
    cluster.Keyspace = keyspace
    cluster.Consistency = gocql.Any

    session, err := cluster.CreateSession()
    if err != nil {
        return nil, err
    }

    return CassConnection{
        dl: dl,
        session: session
    }, nil
}

func (dl *CassDatalayer) EraseDb(keyspace string) error {
    cluster := gocql.NewCluster("127.0.0.1")

    session, err := cluster.CreateSession()
    if err != nil {
        return err
    }

    err = session.Query(`DROP KEYSPACE ` + keyspace + ``).Exec()
    return err
}

func (dl *CassDatalayer) PrepDb(keyspace string) error
    cluster := gocql.NewCluster("127.0.0.1")

    session, err := cluster.CreateSession()
    if err != nil {
        return err
    }

    // Create keyspace.
    err = dl.session.Query(`
            CREATE KEYSPACE ` + keyspace + `
            WITH REPLICATION = {'class' : 'SimpleStrategy', 'replication_factor' : 3}
    `).Exec()
    if err != nil {
        return err
    }

    // Create a new session connecting to that keyspace.
    cluster = gocql.NewCluster("127.0.0.1")
    cluster.Keyspace = keyspace
    cluster.Consistency = gocql.Quorum
    session, err = dl.cluster.CreateSession()
    if err != nil {
        return err
    }

    // Perform all creation queries.
    for _, query := range creationQueries {
        if err := dl.session.Query(query).Exec(); err != nil {
            // Ignore errors (just print them).
            // This allows PrepDB to be used to add new tables.  Eventually, we
            // should come up with a proper migration strategy.
            log.Print(query, "\n", err)
        }
    }
    return nil
}