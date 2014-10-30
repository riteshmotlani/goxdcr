// Copyright (c) 2013 Couchbase, Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
// except in compliance with the License. You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the
// License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing permissions
// and limitations under the License.

// Test for KVFeed, source nozzle in XDCR
package main

import (
	"flag"
	"fmt"
	connector "github.com/Xiaomei-Zhang/goxdcr/connector"
	mcc "github.com/couchbase/gomemcached/client"
	protobuf "github.com/couchbase/indexing/secondary/protobuf"
	"github.com/ysui6888/indexing/secondary/common"
	sp "github.com/ysui6888/indexing/secondary/projector"
	"log"
	"os"
	"time"
	"net/http"
)

import _ "net/http/pprof"

var options struct {
	source_bucket string // source bucket
	connectStr    string //connect string
	kvaddr        string //kvaddr
	maxVbno       int    // maximum number of vbuckets
}

const (
	NUM_DATA = 30 // number of data points to collect before test ends
)

var count int

func argParse() {
	flag.StringVar(&options.connectStr, "connectStr", "127.0.0.1:9000",
		"connection string to source cluster")
	flag.StringVar(&options.source_bucket, "source_bucket", "default",
		"bucket to replicate from")
	flag.IntVar(&options.maxVbno, "maxvb", 8,
		"maximum number of vbuckets")
	flag.StringVar(&options.kvaddr, "kvaddr", "127.0.0.1:12000",
		"kv server address")

	flag.Parse()
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage : %s [OPTIONS] \n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	go func() {
		log.Println("Try to start pprof...")
		err := http.ListenAndServe("localhost:7000", nil)
		if err != nil {
			panic(err)
		} else {
			log.Println("Http server for pprof is started")
		}
	}()

	fmt.Println("Start Testing KVFeed...")
	argParse()
	fmt.Printf("connectStr=%s\n", options.connectStr)
	fmt.Println("Done with parsing the arguments")
	
	for i:=0; i<16; i++ {
	go startKVFeed(options.connectStr, options.kvaddr, options.source_bucket)
	}

	time.Sleep (3 * time.Minute)
}

func mf(err error, msg string) {
	if err != nil {
		log.Fatalf("%v: %v", msg, err)
	}
}

func startKVFeed(cluster, kvaddr, bucketn string) {
	b, err := common.ConnectBucket(cluster, "default", bucketn)
	mf(err, "bucket")

	kvfeed, err := sp.NewKVFeed(kvaddr, "test", "", b, nil)
	kvfeed.SetConnector(NewTestConnector())
	kvfeed.Start(sp.ConstructStartSettingsForKVFeed(constructTimestamp(bucketn)))
	fmt.Println("KVFeed is started")

	timeChan := time.NewTimer(time.Second * 1000).C
loop:
	for {
		select {
		case <-timeChan:
			fmt.Println("Timer expired")
			break loop
		default:
//			if count >= NUM_DATA {
//				break loop
//			}
		}
	}
	kvfeed.Stop()
	fmt.Println("KVFeed is stopped")

	if count < NUM_DATA {
		fmt.Printf("Test failed. Only %v data was received before timer expired.\n", count)
	} else {
		fmt.Println("Test passed. All test data was received as expected before timer expired.")
	}
}

func constructTimestamp(bucketn string) *protobuf.TsVbuuid {
	ts := protobuf.NewTsVbuuid(bucketn, options.maxVbno)
	for i := 0; i < options.maxVbno; i++ {
		ts.Append(uint16(i), 0, 0, 0, 0)
	}
	return ts
}

type TestConnector struct {
	*connector.SimpleConnector
}

func NewTestConnector() *TestConnector {
	tc := new(TestConnector)
	tc.SimpleConnector = new(connector.SimpleConnector)
	return tc
}

func (tc *TestConnector) Forward(data interface{}) error {
	uprEvent := data.(*mcc.UprEvent)
	count++
	fmt.Printf("received %vth upr event with opcode %v and vbno %v\n", count, uprEvent.Opcode, uprEvent.VBucket)
	return nil
}
