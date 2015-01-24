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
	"errors"
	"fmt"
	"github.com/couchbase/cbauth"
	"github.com/couchbase/goxdcr/base"
	"github.com/couchbase/goxdcr/log"
	rm "github.com/couchbase/goxdcr/replication_manager"
	"github.com/couchbase/goxdcr/utils"
)

var ErrorParsingHostInfo = errors.New("Could not parse current host info from server result.")

type XDCRTopologySvc struct {
	adminport    uint16
	xdcrRestPort uint16
	local_proxy_port uint16
	isEnterprise bool
	logger       *log.CommonLogger
}

func NewXDCRTopologySvc(adminport, xdcrRestPort, localProxyPort uint16,
	isEnterprise bool, logger_ctx *log.LoggerContext) (*XDCRTopologySvc, error) {
	top_svc := &XDCRTopologySvc{
		adminport:    adminport,
		xdcrRestPort: xdcrRestPort,
		local_proxy_port: localProxyPort,
		isEnterprise: isEnterprise,
		logger:       log.NewLogger("XDCRTopologyService", logger_ctx),
	}
	return top_svc, nil
}

func (top_svc *XDCRTopologySvc) MyHost() (string, error) {
	return top_svc.getHostName()
}

func (top_svc *XDCRTopologySvc) MyHostAddr() (string, error) {
	return top_svc.getHostAddr()
}

func (top_svc *XDCRTopologySvc) MyMemcachedAddr() (string, error) {
	port, err := top_svc.getHostMemcachedPort()
	if err != nil {
		return "", err
	}
	
	hostName, err := top_svc.getHostName()
	if err != nil {
		return "", err
	}
	
	return utils.GetHostAddr(hostName, port), nil
}

func (top_svc *XDCRTopologySvc) MyAdminPort() (uint16, error) {
	return top_svc.adminport, nil
}

func (top_svc *XDCRTopologySvc) MyKVNodes() ([]string, error) {
	// as of now each xdcr instance is responsible for only one kv node
	nodes := make([]string, 1)
	// get the actual hostname used in server list and server vb map
	memcachedAddr, err := top_svc.MyMemcachedAddr()
	if err != nil {
		return nil, err
	}
	nodes[0] = memcachedAddr
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

// get information about current node from nodeService at /pools/nodes
func (top_svc *XDCRTopologySvc) getHostInfo() (map[string]interface{}, error) {
	hostAddr := "http://" + utils.GetHostAddr(base.LocalHostName, top_svc.adminport)
	var nodesInfo map[string]interface{}
	if hostAddr == "" {
		panic("hostAddr can't be empty")
	}
	err, statusCode := utils.QueryRestApi(hostAddr, base.NodesPath, false, base.MethodGet, "", nil, 0, &nodesInfo, top_svc.logger)
	if err != nil || statusCode != 200 {
		return nil, errors.New(fmt.Sprintf("Failed on calling %v, err=%v, statusCode=%v", base.NodesPath, err, statusCode))
	}
	// get node list from the map
	nodes, ok := nodesInfo[base.NodesKey]
	if !ok {
		// should never get here
		top_svc.logger.Errorf("no nodes")
		return nil, ErrorParsingHostInfo
	}

	nodeList, ok := nodes.([]interface{})
	if !ok {
		// should never get here
		return nil, ErrorParsingHostInfo
	}

	for _, node := range nodeList {
		nodeInfoMap, ok := node.(map[string]interface{})
		if !ok {
			// should never get here
			return nil, ErrorParsingHostInfo
		}

		thisNode, ok := nodeInfoMap[base.ThisNodeKey]
		if ok {
			thisNodeBool, ok := thisNode.(bool)
			if !ok {
				// should never get here
				return nil, ErrorParsingHostInfo
			}
			if thisNodeBool {
				// found current node
				return nodeInfoMap, nil
			}
		}
	}

	return nil, ErrorParsingHostInfo
}

// get address of current node
func (top_svc *XDCRTopologySvc) getHostAddr() (string, error) {
	nodeInfoMap, err := top_svc.getHostInfo()
	if err != nil {
		return "", err
	}

	hostAddr, ok := nodeInfoMap[base.HostNameKey]
	if !ok {
		// should never get here
		return "", ErrorParsingHostInfo
	}
	hostAddrStr, ok := hostAddr.(string)
	if !ok {
		// should never get here
		return "", ErrorParsingHostInfo
	}
	return hostAddrStr, nil
}

// get name of current node
func (top_svc *XDCRTopologySvc) getHostName() (string, error) {
	hostAddrStr, err := top_svc.getHostAddr()
	if err != nil {
		return "", nil
	}
	hostname := utils.GetHostName(hostAddrStr)
	return hostname, nil
}

// get memcached port of current node
func (top_svc *XDCRTopologySvc) getHostMemcachedPort() (uint16, error) {
	nodeInfoMap, err := top_svc.getHostInfo()
	if err != nil {
		return 0, err
	}

	ports, ok := nodeInfoMap[base.PortsKey]
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}
	portsMap, ok := ports.(map[string]interface{})
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}

	directPort, ok := portsMap[base.DirectPortKey]
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}
	directPortFloat, ok := directPort.(float64)
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}

	return uint16(directPortFloat), nil
}

// implements base.ClusterConnectionInfoProvider
func (top_svc *XDCRTopologySvc) MyConnectionStr() (string, error) {
	host := base.LocalHostName
	return utils.GetHostAddr(host, top_svc.adminport), nil
}

func (top_svc *XDCRTopologySvc) MyCredentials() (string, string, error) {
	connStr, err := top_svc.MyConnectionStr()
	if err != nil {
		return "", "", err
	}
	if connStr == "" {
		panic("connStr == ")
	}
	return cbauth.GetHTTPServiceAuth(connStr)
}

func (top_svc *XDCRTopologySvc) MyProxyPort() (uint16, error) {
	return top_svc.local_proxy_port, nil
}
