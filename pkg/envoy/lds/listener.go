package lds

import (
	xds_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	xds_listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	xds_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	xds_tcp_proxy "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/openservicemesh/osm/pkg/service"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/envoy"
)

const (
	inboundListenerName           = "inbound-listener"
	outboundListenerName          = "outbound-listener"
	prometheusListenerName        = "inbound-prometheus-listener"
	outboundEgressFilterChainName = "outbound-egress-filter-chain"
	singleIpv4Mask                = 32
)

func (lb *listenerBuilder) newOutboundListener() (*xds_listener.Listener, error) {
	serviceFilterChains := lb.getOutboundFilterChainPerUpstream()

	listener := &xds_listener.Listener{
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
		},
	}

	// Create filter chain for egress if egress is enabled
	// This filterchain matches any traffic not filtered by allow rules, it will be treated as egress
	// traffic when enabled
	if lb.cfg.IsEgressEnabled() {
		egressFilterChgain, err := buildEgressFilterChain()
		if err != nil {
			log.Error().Err(err).Msgf("Error getting filter chain for Egress")
			return nil, err
		}
		listener.DefaultFilterChain = egressFilterChgain
	}

	if len(listener.FilterChains) == 0 && listener.DefaultFilterChain == nil {
		// Programming a listener with no filter chains is an error.
		// It is possible for the outbound listener to have no filter chains if
		// there are no allowed upstreams for this proxy and egress is disabled.
		// In this case, return a nil filter chain so that it doesn't get programmed.
		return nil, nil
	}

	return listener, nil
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
