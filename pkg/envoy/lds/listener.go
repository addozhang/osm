package lds

import (
	xds_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	xds_listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	xds_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	xds_tcp_proxy "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/envoy"
	"github.com/openservicemesh/osm/pkg/kubernetes"
	"github.com/openservicemesh/osm/pkg/service"
)

const (
	inboundListenerName           = "inbound-listener"
	outboundListenerName          = "outbound-listener"
	prometheusListenerName        = "inbound-prometheus-listener"
	outboundEgressFilterChainName = "outbound-egress-filter-chain"
	singleIpv4Mask                = 32
)

func (lb *listenerBuilder) newOutboundListener() (*xds_listener.Listener, error) {
	serviceFilterChains, err := lb.getOutboundFilterChains()
	if err != nil {
		log.Error().Err(err).Msgf("Error getting filter chains for outbound listener")
		return nil, err
	}

	if len(serviceFilterChains) == 0 {
		log.Info().Msgf("No filterchains for outbound services. Not programming Outbound listener.")
		return nil, nil
	}

	return &xds_listener.Listener{
		Name:             outboundListenerName,
		Address:          envoy.GetAddress(constants.WildcardIPAddr, constants.EnvoyOutboundListenerPort),
		TrafficDirection: xds_core.TrafficDirection_OUTBOUND,
		FilterChains:     serviceFilterChains,
		ListenerFilters: []*xds_listener.ListenerFilter{
			{
				// The OriginalDestination ListenerFilter is used to redirect traffic
				// to its original destination.
				Name: wellknown.OriginalDestination,
			},
			/*{ 	//TODO local fix to support non-http traffic proxy, or it will result in filter timeout (default 15s)
				// The HttpInspector ListenerFilter is used to inspect plaintext traffic
				// for HTTP protocols.
				Name: wellknown.HttpInspector,
			},*/
		},
	}, nil
}

func newInboundListener() *xds_listener.Listener {
	return &xds_listener.Listener{
		Name:             inboundListenerName,
		Address:          envoy.GetAddress(constants.WildcardIPAddr, constants.EnvoyInboundListenerPort),
		TrafficDirection: xds_core.TrafficDirection_INBOUND,
		FilterChains:     []*xds_listener.FilterChain{},
		ListenerFilters: []*xds_listener.ListenerFilter{
			{
				Name: wellknown.TlsInspector,
			},
			{
				// The OriginalDestination ListenerFilter is used to restore the original destination address
				// as opposed to the listener's address upon iptables redirection.
				// This enables inbound filter chain matching on the original destination address (ip, port).
				Name: wellknown.OriginalDestination,
			},
		},
	}
}

func buildPrometheusListener(connManager *xds_hcm.HttpConnectionManager) (*xds_listener.Listener, error) {
	marshalledConnManager, err := ptypes.MarshalAny(connManager)
	if err != nil {
		log.Error().Err(err).Msgf("Error marshalling HttpConnectionManager object")
		return nil, err
	}

	return &xds_listener.Listener{
		Name:             prometheusListenerName,
		TrafficDirection: xds_core.TrafficDirection_INBOUND,
		Address:          envoy.GetAddress(constants.WildcardIPAddr, constants.EnvoyPrometheusInboundListenerPort),
		FilterChains: []*xds_listener.FilterChain{
			{
				Filters: []*xds_listener.Filter{
					{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &xds_listener.Filter_TypedConfig{
							TypedConfig: marshalledConnManager,
						},
					},
				},
			},
		},
	}, nil
}

func buildInboundPassthroughFilterChain(proxyService service.MeshService, port uint32) (*xds_listener.FilterChain, error) {
	tcpProxy := &xds_tcp_proxy.TcpProxy{
		StatPrefix:       envoy.OutboundPassthroughCluster,
		ClusterSpecifier: &xds_tcp_proxy.TcpProxy_Cluster{Cluster: envoy.OutboundPassthroughCluster},
		//ClusterSpecifier: &xds_tcp_proxy.TcpProxy_Cluster{Cluster: envoy.GetLocalClusterNameForService(proxyService)},
	}
	marshalledTCPProxy, err := ptypes.MarshalAny(tcpProxy)
	if err != nil {
		log.Error().Err(err).Msgf("Error marshalling TcpProxy object for egress HTTPS filter chain")
		return nil, err
	}

	return &xds_listener.FilterChain{
		FilterChainMatch: &xds_listener.FilterChainMatch{
			DestinationPort: &wrapperspb.UInt32Value{
				Value: port,
			},
		},
		Filters: []*xds_listener.Filter{
			{
				Name:       wellknown.TCPProxy,
				ConfigType: &xds_listener.Filter_TypedConfig{TypedConfig: marshalledTCPProxy},
			},
		},
	}, nil
}

func buildEgressFilterChain() (*xds_listener.FilterChain, error) {
	tcpProxy := &xds_tcp_proxy.TcpProxy{
		StatPrefix:       envoy.OutboundPassthroughCluster,
		ClusterSpecifier: &xds_tcp_proxy.TcpProxy_Cluster{Cluster: envoy.OutboundPassthroughCluster},
	}
	marshalledTCPProxy, err := ptypes.MarshalAny(tcpProxy)
	if err != nil {
		log.Error().Err(err).Msgf("Error marshalling TcpProxy object for egress HTTPS filter chain")
		return nil, err
	}

	return &xds_listener.FilterChain{
		Name: outboundEgressFilterChainName,
		Filters: []*xds_listener.Filter{
			{
				Name:       wellknown.TCPProxy,
				ConfigType: &xds_listener.Filter_TypedConfig{TypedConfig: marshalledTCPProxy},
			},
		},
	}, nil
}

func (lb *listenerBuilder) getOutboundFilterChains() ([]*xds_listener.FilterChain, error) {
	// Create filter chain for upstream services
	filterChains := lb.getOutboundFilterChainPerUpstream()
	//var filterChains []*xds_listener.FilterChain

	//local fix to add common filter for match all pods ip with Pod CIDR
	commonFilter, err := lb.getOutboundHTTPFilter()
	if err != nil {
		log.Error().Err(err).Msg("Error getting common filter")
		return nil, err
	}
	filterChains = append(filterChains, &xds_listener.FilterChain{
		Name:    "internal-cluster",
		Filters: []*xds_listener.Filter{commonFilter},
		FilterChainMatch: &xds_listener.FilterChainMatch{
			PrefixRanges: []*xds_core.CidrRange{
				{
					AddressPrefix: "10.128.0.0", //TODO pod CIDR
					PrefixLen: &wrapperspb.UInt32Value{
						Value: 12,
					},
				},
			},
		},
	})

	// Create filter chain for egress if egress is enabled
	// This filterchain matches any traffic not filtered by allow rules, it will be treated as egress
	// traffic when enabled
	if lb.cfg.IsEgressEnabled() {
		egressFilterChgain, err := buildEgressFilterChain()
		if err != nil {
			log.Error().Err(err).Msgf("Error getting filter chain for Egress")
			return nil, err
		}

		filterChains = append(filterChains, egressFilterChgain)
	}

	return filterChains, nil
}

// getOutboundFilterChainPerUpstream returns a list of filter chains corresponding to upstream services
func (lb *listenerBuilder) getOutboundFilterChainPerUpstream() []*xds_listener.FilterChain {
	var filterChains []*xds_listener.FilterChain

	outboundSvc := lb.meshCatalog.ListAllowedOutboundServicesForIdentity(lb.svcAccount)
	if len(outboundSvc) == 0 {
		log.Debug().Msgf("Proxy with identity %s does not have any allowed upstream services", lb.svcAccount)
		return filterChains
	}

	var dstServicesSet map[service.MeshService]struct{} = make(map[service.MeshService]struct{}) // Set, avoid duplicates
	// Transform into set, when listing apex services we might face repetitions
	for _, meshSvc := range outboundSvc {
		dstServicesSet[meshSvc] = struct{}{}
	}

	// Getting apex services referring to the outbound services
	// We get possible apexes which could traffic split to any of the possible
	// outbound services
	splitServices := lb.meshCatalog.GetSMISpec().ListTrafficSplitServices()
	for _, svc := range splitServices {
		for _, outSvc := range outboundSvc {
			if svc.Service == outSvc {
				rootServiceName := kubernetes.GetServiceFromHostname(svc.RootService)
				rootMeshService := service.MeshService{
					Namespace: outSvc.Namespace,
					Name:      rootServiceName,
				}

				// Add this root service into the set
				dstServicesSet[rootMeshService] = struct{}{}
			}
		}
	}

	// Iterate all destination services
	for upstream := range dstServicesSet {
		// Construct HTTP filter chain
		if httpFilterChain, err := lb.getOutboundHTTPFilterChainForService(upstream); err != nil {
			log.Error().Err(err).Msgf("Error constructing outbound HTTP filter chain for upstream service %s on proxy with identity %s", upstream, lb.svcAccount)
		} else {
			filterChains = append(filterChains, httpFilterChain)
		}

		// Construct TCP filter chain
		/* TODO local fix to disable outbound tcp filter chain
		if tcpFilterChain, err := lb.getOutboundTCPFilterChainForService(upstream); err != nil {
			log.Error().Err(err).Msgf("Error constructing outbound TCP filter chain for upstream service %s on proxy with identity %s", upstream, lb.svcAccount)
		} else {
			filterChains = append(filterChains, tcpFilterChain)
		}*/
	}

	return filterChains
}
