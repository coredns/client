package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/coredns/client"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
)

func main() {
	var (
		endpoint string
		qname    string
		qtype    uint16
		verbose  bool
		insecure bool
		watch    bool
		cert     string
		key      string
		ca       string
	)

	flag.BoolVar(&verbose, "v", false, "Log verbosely")
	flag.BoolVar(&insecure, "insecure", false, "Don't use TLS")
	flag.BoolVar(&watch, "w", false, "Start a watch")
	flag.StringVar(&cert, "cert", "", "TLS cert PEM file path")
	flag.StringVar(&key, "key", "", "TLS key PEM file path")
	flag.StringVar(&ca, "ca", "", "TLS ca cert PEM file path")
	flag.StringVar(&endpoint, "server", "localhost:5553", "server host:port")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 || len(args) > 2 {
		panic(fmt.Errorf("Usage: dnsgrpc [flags] <name> [type]"))
	}
	qname = args[0]
	qtype = dns.TypeA
	if len(args) > 1 {
		qtype = dns.StringToType[strings.ToUpper(args[1])]
	}

	if verbose {
		a := "Querying for"
		if watch {
			a = "Querying and setting a watch for"
		}
		fmt.Printf("%s %s records for %s on server %s\n", a, dns.TypeToString[qtype], qname, endpoint)
	}

	c, err := client.NewClient(endpoint, cert, key, ca, []grpc.DialOption{})
	if err != nil {
		panic(err)
	}

	m, err := c.QueryNameAndType(qname, qtype)
	if err != nil {
		panic(err)
	}
	printMsg(verbose, m)

	if watch {
		w, err := c.WatchNameAndType(qname, qtype)
		if err != nil {
			panic(err)
		}
		if verbose {
			fmt.Printf("Started watch %v", w)
		}

		for msg := range w.Msgs {
			if msg.Msg != nil {
				printMsg(verbose, msg.Msg)
			}
			if msg.Err != "" {
				fmt.Printf("Error: %v\n", msg.Err)
			}
			if msg.End {
				fmt.Println("Watch ended by server. Stopping.")
				return
			}
		}
	}
}

func printMsg(verbose bool, m *dns.Msg) {
	if verbose {
		fmt.Printf("%s\n\n\n", m)
		return
	}

	if m == nil {
		fmt.Printf("<nil>\n\n\n")
		return
	}

	if m.MsgHdr.Rcode > 0 {
		qname := m.Question[0].Name
		qtype := dns.TypeToString[m.Question[0].Qtype]
		rcode := dns.RcodeToString[m.MsgHdr.Rcode]

		fmt.Printf("%s %s %s\n\n\n", qname, qtype, rcode)
		return
	}

	for _, rr := range m.Answer {
		fmt.Printf("%s\n", rr.String())
	}
	for _, rr := range m.Extra {
		fmt.Printf("%s\n", rr.String())
	}
	fmt.Printf("\n\n")
}
