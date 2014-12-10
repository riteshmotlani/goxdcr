// Copyright (c) 2013 Couchbase, Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
// except in compliance with the License. You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the
// License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing permissions
// and limitations under the License.

package service_impl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"	
	"github.com/couchbase/goxdcr/base"
	"github.com/couchbase/goxdcr/utils"
	"github.com/couchbase/goxdcr/log"
	rm "github.com/couchbase/goxdcr/replication_manager"
)

var ErrorRetrievingHostInfo = errors.New("Could not parse current host name from server result.")

type XDCRTopologySvc struct {
	adminport  uint16
	xdcrRestPort  uint16
	username   string
	password   string
	isEnterprise  bool
	logger      *log.CommonLogger
}

func NewXDCRTopologySvc(username, password string, adminport, xdcrRestPort uint16, 
    isEnterprise bool, logger_ctx *log.LoggerContext) (*XDCRTopologySvc, error) {
	top_svc := &XDCRTopologySvc{
					username:  username,
					password:  password,
					adminport:  adminport ,
					xdcrRestPort:  xdcrRestPort,
					isEnterprise: isEnterprise,
					logger:    log.NewLogger("XDCRTopologyService", logger_ctx),
					}
	return top_svc, nil
}

func (top_svc *XDCRTopologySvc) MyHost() (string, error) {
	return base.LocalHostName, nil
}

func (top_svc *XDCRTopologySvc) MyAdminPort() (uint16, error) {
	return top_svc.adminport, nil
}

func (top_svc *XDCRTopologySvc) MyKVNodes() ([]string, error) {
	// as of now each xdcr instance is responsible for only one kv node
	nodes := make([]string, 1)
	// get the actual hostname used in server list and server vb map
	hostname, err := top_svc.getHostName()
	if err != nil {
		return nil, err
	}
	nodes[0] = hostname
	return nodes, nil
}

func (top_svc *XDCRTopologySvc) XDCRTopology() (map[string]uint16, error) {
	retmap := make(map[string]uint16)
	serverList, err := rm.ClusterInfoService().GetServerList(top_svc, "default")
	if err != nil {
		return nil, err
	}
	for _, server := range serverList {
		serverName := utils.GetHostName(server)
		retmap[serverName] = top_svc.xdcrRestPort
	}
	return retmap, nil
}

func (top_svc *XDCRTopologySvc) IsMyClusterEnterprise() (bool, error) {
	return top_svc.isEnterprise, nil
}

// currently not used and not implemented
func (top_svc *XDCRTopologySvc) XDCRCompToKVNodeMap() (map[string][]string, error) {
	retmap := make(map[string][]string)
	return retmap, nil
}

// get hostname from nodeService at /pools/nodes
func (top_svc *XDCRTopologySvc) getHostName() (string, error) {
	hostAddr := utils.GetHostAddr(base.LocalHostName, top_svc.adminport)
	url := fmt.Sprintf("http://%s:%s@%s%s", top_svc.username, top_svc.password, hostAddr, base.NodesPath)
	request, err := http.NewRequest(base.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	
	response, err := utils.SendHttpRequest(request)
	if err != nil {
		return "", err
	}
	
	defer response.Body.Close()
	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	
	//  /pools/nodes returns a map
	var nodesInfo map[string]interface{}
	err = json.Unmarshal(bodyBytes, &nodesInfo)
	if err != nil {
		return "", err
	}
	
	// get node list from the map 
	nodes, ok := nodesInfo[base.NodesKey]
	if !ok {
		// should never get here
		top_svc.logger.Errorf("no nodes")
		return "", ErrorRetrievingHostInfo
	}
	
	nodeList, ok := nodes.([]interface{})
	if !ok {
		// should never get here
		return "", ErrorRetrievingHostInfo
	}
	
	for _, node := range nodeList {
		nodeInfoMap, ok := node.(map[string]interface{})
		if !ok {
			// should never get here
			return "", ErrorRetrievingHostInfo
		}
		
		thisNode, ok := nodeInfoMap[base.ThisNodeKey]
		if ok {
			thisNodeBool, ok := thisNode.(bool)
			if !ok {
				// should never get here
				return "", ErrorRetrievingHostInfo
			}
			if thisNodeBool {
				// found current node
				hostAddr, ok := nodeInfoMap[base.HostNameKey]
				if !ok {
					// should never get here
					return "", ErrorRetrievingHostInfo
				}
				hostAddrStr, ok := hostAddr.(string)
				if !ok {
					// should never get here
					return "", ErrorRetrievingHostInfo
				}
				hostname := utils.GetHostName(hostAddrStr)
				top_svc.logger.Infof("MyHost() returned %v\n", hostname)
				return hostname, nil
			}
		}
	}
	
	return "", ErrorRetrievingHostInfo
}

// implements base.ClusterConnectionInfoProvider
func (top_svc *XDCRTopologySvc)	MyConnectionStr() string {
	host, err := top_svc.MyHost()
	if err != nil {
		// should never get here
		return ""
	}
	return utils.GetHostAddr(host, top_svc.adminport)
}

func (top_svc *XDCRTopologySvc)	MyUsername()  string {
	return top_svc.username
}

func (top_svc *XDCRTopologySvc)	MyPassword()  string {
	return top_svc.password
}