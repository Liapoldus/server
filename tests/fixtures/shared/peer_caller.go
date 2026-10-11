package shared

import (
	"context"
	"errors"
	"io"

	protocolpeer "github.com/Liapoldus/pluginprotocol/v3/presentation/peer"
)

// PeerTarget and PeerCaller are deterministic test adapters that stand in for
// a Core-issued peer directory. Production Server code never receives this
// static endpoint map.
type PeerTarget struct {
	Endpoint string
	Security protocolpeer.SecurityConfig
}

type PeerCaller struct {
	Targets map[string]PeerTarget
}

func (caller PeerCaller) Call(ctx context.Context, target, method string, payload []byte) ([]byte, error) {
	client, err := caller.dial(ctx, target)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()
	result, err := client.Call(ctx, protocolpeer.Method(method), payload)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), result.Payload...), nil
}

func (caller PeerCaller) OpenStream(ctx context.Context, target, method string) (protocolpeer.Stream, io.Closer, error) {
	client, err := caller.dial(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	stream, err := client.OpenStream(ctx, protocolpeer.Method(method))
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return stream, client, nil
}

func (caller PeerCaller) dial(ctx context.Context, target string) (protocolpeer.Client, error) {
	resolved, ok := caller.Targets[target]
	if !ok || resolved.Endpoint == "" {
		return nil, errors.New("test peer target unavailable")
	}
	registry, err := protocolpeer.NewRegistry().Build()
	if err != nil {
		return nil, err
	}
	return protocolpeer.Dial(ctx, protocolpeer.ClientConfig{
		Network:  protocolpeer.NetworkConfig{Carrier: protocolpeer.CarrierTCP, Endpoint: resolved.Endpoint},
		Security: resolved.Security,
		Handler:  registry,
	})
}
