package rds

import (
	"fmt"
	xds_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/openservicemesh/osm/pkg/catalog"
	"github.com/openservicemesh/osm/pkg/configurator"
	"github.com/openservicemesh/osm/pkg/envoy"
	"github.com/openservicemesh/osm/pkg/service"
)

func processInBoundRouterForAccessViaIP(catalog catalog.MeshCataloger, proxy *envoy.Proxy, err error,
	inboundRouteConfig *xds_route.RouteConfiguration, configurator configurator.Configurator,
	svcList []service.MeshService, proxyServiceName service.MeshService) error {
	//insert pod ip and port pair as host to inbound route
	//pod, err := catalog.GetPodFromCertificate(proxy.GetCommonName())
	//if err != nil {
	//	return err
	//}
	for _, vh := range inboundRouteConfig.VirtualHosts {
		vh.Domains = append(vh.Domains, fmt.Sprintf("*:%d", 8080))
		// handling multi service case
		if len(svcList) > 0 {
			for _, meshService := range svcList {
				if meshService != proxyServiceName {
					vh.Domains = append(vh.Domains, meshService.Name)
					vh.Domains = append(vh.Domains, fmt.Sprintf("%s.%s", meshService.Name, meshService.Namespace))
				}
			}
		}
		break
	}
	return nil
}
