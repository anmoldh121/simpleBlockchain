package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	net "github.com/libp2p/go-libp2p-core/network"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	mplex "github.com/libp2p/go-libp2p-mplex"
	noise "github.com/libp2p/go-libp2p-noise"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	tcp "github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	log "github.com/sirupsen/logrus"
)

var (
	bootstrapNode = []string{
		"/ip4/13.59.233.151/tcp/4000/p2p/QmQnAZsyiJSovuqg8zjP3nKdm6Pwb75Mpn8HnGyD5WYZ15",
	}
)

func CreateHost(ctx context.Context) (host.Host, error) {
	var r io.Reader
	r = rand.Reader

	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	var listenPort = "0"

	transport := libp2p.ChainOptions(
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(ws.New),
	)
	muxer := libp2p.ChainOptions(
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
	)

	listenAddr := libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", listenPort), fmt.Sprintf("/ip4/0.0.0.0/tcp/%s/ws", listenPort))

	log.Info("Creating Host at port ", listenPort)

	host, err := libp2p.New(
		ctx,
		transport,
		listenAddr,
		muxer,
		libp2p.Security(noise.ID, noise.New),
		libp2p.Identity(prvKey),
	)
	if err != nil {
		return nil, err
	}
	host.SetStreamHandler("/chat/1.0.0", StramHandler)
	return host, nil
}

func readData(rw *bufio.ReadWriter) {
	for {
		str, err := rw.ReadString('\n')
		if err != nil {
			log.Error("Error reading from buffer")
		}
		if str == "" {
			return
		}
		if str != "\n" {
			fmt.Printf("\x1b[32m%s\x1b[0m> ", str)
		}
	}
}

func writeData(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(">")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			log.Error("Error reading from stdin")
		}
		_, err = rw.WriteString(fmt.Sprintf("%s\n", sendData))
		if err != nil {
			log.Error("Error writing buffer")
		}
		err = rw.Flush()
		if err != nil {
			log.Error("Error while flushing buffer")
		}
	}
}

func StramHandler(stream net.Stream) {
	log.Info("Stream connected")
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	go readData(rw)
	go writeData(rw)
}

func setupDiscovery(ctx context.Context, host host.Host) error {
	kademliaDHT, err := dht.New(ctx, host)
	if err != nil {
		return err
	}
	log.Info("Bootstrapping node")
	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, p := range bootstrapNode {
		in, _ := ma.NewMultiaddr(p)
		peerInfo, err := peerstore.InfoFromP2pAddr(in)
		if err != nil {
			panic(err)
		}
		// peerInfo, _ := peer.AddrInfoFromP2pAddr(peerAdr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *peerInfo); err != nil {
				log.Warning(err)
			} else {
				log.Info("Connection established with bootstrap node: ", *peerInfo)
			}
		}()
	}
	wg.Wait()

	log.Info("Announcing ourselves...")
	routingDiscovery := discovery.NewRoutingDiscovery(kademliaDHT)
	discovery.Advertise(ctx, routingDiscovery, "randezvous")
	log.Info("Successfully announced!")

	log.Info("Searching for other peers...")
	peerChan, err := routingDiscovery.FindPeers(ctx, "randezvous")
	if err != nil {
		return err
	}

	for peer := range peerChan {
		if peer.ID == host.ID() {
			continue
		}
		log.Debug("Found peer: ", peer)
		log.Debug("Connecting to peer: ", peer)
		err := host.Connect(context.Background(), peer)
		if err != nil {
			log.Warning("Error connecting to peer ", err)
			continue
		} else {
			stream, err := host.NewStream(ctx, peer.ID, "/chat/1.0.0")
			if err != nil {
				log.Warning("Error in creating stream")
			}
			StramHandler(stream)
		}
		log.Info("Connected to peer", peer)
	}

	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	basicHost, err := CreateHost(ctx)
	if err != nil {
		log.Panic(err)
	}

	log.Info("Host created we are: ", basicHost.ID())
	log.Info(basicHost.Addrs())

	err = setupDiscovery(ctx, basicHost)
	if err != nil {
		log.Panic(err)
	}
	select {}
}
