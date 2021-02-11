package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"

	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht/dual"
	noise "github.com/libp2p/go-libp2p-noise"
	// quic "github.com/libp2p/go-libp2p-quic-transport"
	tcp "github.com/libp2p/go-tcp-transport"
	ma "github.com/multiformats/go-multiaddr"
	log "github.com/sirupsen/logrus"
)

const PORT = "0"

var (
	kahdemiaDHT *dht.DHT
	serviceNode = []string{
		// "/ip4/127.0.0.1/udp/4000/quic/p2p/QmVbcMycaK8ni5CeiM7JRjBRAdmwky6dQ6KcoxLesZDPk9",
		"/ip4/127.0.0.1/tcp/4000/p2p/QmQnAZsyiJSovuqg8zjP3nKdm6Pwb75Mpn8HnGyD5WYZ15",
	}
)

func CreateHost(ctx context.Context) (host.Host, error) {
	var r io.Reader
	r = rand.Reader
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/udp/%s/quic", PORT), fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", PORT)),
		// libp2p.Transport(quic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Identity(priv),
		libp2p.Security(noise.ID, noise.New),
		libp2p.NATPortMap(),
	}

	basicHost, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return basicHost, nil
}

func setupDiscovery(ctx context.Context, host host.Host) error {
	log.Info("Bootstrapping DHT")
	kahdemiaDHT, err := dht.New(ctx, host)
	if err != nil {
		return err
	}
	if err := kahdemiaDHT.Bootstrap(ctx); err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, peerAddr := range serviceNode {
		maddr := ma.StringCast(peerAddr)
		peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *peerInfo); err != nil {
				log.Warn("Can not connect to service node")
			} else {
				log.Info("Connection established with service node: ", *peerInfo)
			}
		}()
	}
	wg.Wait()

	log.Info("Announcing ourselves")
	routingDiscovery := discovery.NewRoutingDiscovery(kahdemiaDHT)
	discovery.Advertise(ctx, routingDiscovery, "meethere")
	log.Info("Successfully announced")
	log.Info("peers", host.Peerstore().Peers())

	log.Debug("Searching for other peers")
	peerChan, err := routingDiscovery.FindPeers(ctx, "meethere")
	if err != nil {
		return err
	}
	log.Info("Length of peer discovery: ", len(peerChan))
	return nil
}

func main() {
	ctx := context.Background()
	host, err := CreateHost(ctx)
	if err != nil {
		log.Error("error creating host", err)
	}
	log.Info("Host created: ", host.ID(), host.Addrs())

	err = setupDiscovery(ctx, host)
	if err != nil {
		log.Error("Error in setting up discovery ", err)
	}

	select {}
}
