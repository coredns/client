package client

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/coredns/coredns/pb"
	"github.com/coredns/coredns/plugin/pkg/tls"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	creds "google.golang.org/grpc/credentials"
)

// Client provides a convenient interface to a gRPC-based DNS service.
type Client struct {
	pbClient pb.DnsServiceClient
}

// Msg holds a message from the server
type Msg struct {
	Msg *dns.Msg
	Err string
	End bool
}

// Watch is used to track access results from watching a particular query.
type Watch struct {
	WatchID int64
	Msgs    chan *Msg
	stream  pb.DnsService_WatchClient
	client  *Client
}

// NewClient establishes a connection to a server and returns a pointer to a Client.
func NewClient(endpoint, cert, key, ca string, dialOpts []grpc.DialOption) (*Client, error) {
	var tlsargs []string
	if cert != "" {
		tlsargs = append(tlsargs, cert)
	}

	if key != "" {
		tlsargs = append(tlsargs, key)
	}

	if ca != "" {
		tlsargs = append(tlsargs, ca)
	}

	if len(tlsargs) == 0 {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	} else {
		tlsConfig, err := tls.NewTLSConfigFromArgs(tlsargs...)
		if err != nil {
			return nil, err
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds.NewTLS(tlsConfig)))
	}
	conn, err := grpc.Dial(endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &Client{pbClient: pb.NewDnsServiceClient(conn)}, nil
}

// Query performs a query using the gRPC DNS server
func (c *Client) Query(req *dns.Msg) (*dns.Msg, error) {
	msg, err := req.Pack()
	if err != nil {
		return nil, err
	}

	reply, err := c.pbClient.Query(context.Background(), &pb.DnsPacket{Msg: msg})
	if err != nil {
		return nil, err
	}
	d := new(dns.Msg)
	err = d.Unpack(reply.Msg)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// QueryNameAndType is a convenience function that queries by name and type, via gRPC.
func (c *Client) QueryNameAndType(qname string, qtype uint16) (*dns.Msg, error) {
	m := &dns.Msg{}
	m.SetQuestion(dns.Fqdn(qname), qtype)

	return c.Query(m)
}

// Watch requests that the server push change notifications to this client for a
// specific query.
func (c *Client) Watch(req *dns.Msg) (*Watch, error) {
	p, err := req.Pack()
	if err != nil {
		return nil, err
	}

	query := &pb.DnsPacket{Msg: p}
	cr := &pb.WatchRequest_CreateRequest{CreateRequest: &pb.WatchCreateRequest{Query: query}}

	stream, err := c.pbClient.Watch(context.Background())
	if err != nil {
		return nil, err
	}

	if err = stream.Send(&pb.WatchRequest{RequestUnion: cr}); err != nil {
		return nil, err
	}

	in, err := stream.Recv()
	if err == io.EOF {
		return nil, fmt.Errorf("server returned EOF after attempt to create watch")
	}
	if !in.Created {
		return nil, fmt.Errorf("unexpected non-created response from server: %v", in)
	}
	w := &Watch{WatchID: in.WatchId, Msgs: make(chan *Msg), stream: stream, client: c}
	go func() {
		for {
			in, err := w.stream.Recv()
			if err == io.EOF {
				close(w.Msgs)
				return
			}
			if err != nil {
				log.Printf("[ERROR] Watch %d failed to receive from gRPC stream: %s\n", w.WatchID, err)
				close(w.Msgs)
				return
			}

			if in.Err != "" {
				log.Printf("[ERROR] Watch %d got error from server: %v\n", w.WatchID, in.Err)
				w.Msgs <- &Msg{Err: in.Err}
				close(w.Msgs)
				return
			}

			if in.Created {
				log.Printf("[ERROR] Watch %d unexpected created response from server: %v\n", w.WatchID, in)
				close(w.Msgs)
				return
			}

			if in.Canceled {
				log.Printf("Watch %d canceled by server: %v\n", w.WatchID, in)
				w.Msgs <- &Msg{End: true}
				close(w.Msgs)
				return
			}

			r, err := w.client.Query(req)
			if err != nil {
				log.Printf("[ERROR] Error querying for changes: %s\n", err)
				close(w.Msgs)
				return
			}
			w.Msgs <- &Msg{Msg: r}
		}
	}()

	return w, nil
}

// WatchNameAndType is a convenience function to setup a watch by name and type.
func (c *Client) WatchNameAndType(qname string, qtype uint16) (*Watch, error) {
	m := &dns.Msg{}
	m.SetQuestion(dns.Fqdn(qname), qtype)

	return c.Watch(m)
}

// Stop will cancel the watch in the server, so no further updates will be sent
// for that particular query.
func (w *Watch) Stop() error {
	cr := &pb.WatchRequest_CancelRequest{CancelRequest: &pb.WatchCancelRequest{WatchId: w.WatchID}}
	return w.stream.Send(&pb.WatchRequest{RequestUnion: cr})
}
