package helixdns

import (
  "encoding/json"
  "github.com/miekg/dns"
  "log"
  "net"
  "strconv"
  "strings"
)

type HelixServer struct {
  Port   int
  Client Client
}

func Server(port int, etcdurl string) *HelixServer {
  return &HelixServer{ Port: port, Client: NewEtcdClient(etcdurl) }
}

func (s HelixServer) Start() {
  server := &dns.Server{
    Addr:         ":"+strconv.Itoa(s.Port),
    Net:          "udp",
    Handler:      dns.HandlerFunc(s.Handler),
    ReadTimeout:  10,
    WriteTimeout: 10,
  }

  log.Print("Starting server...")

  server.ListenAndServe()
}

func (s HelixServer) getResponse(q dns.Question) (Response, error) {
  addr := dns.SplitDomainName(q.Name)
  path := []string{"helix"}

  for i := len(addr) - 1; i >= 0; i-- {
    path = append(path, addr[i])
  }

  path = append(path, dns.TypeToString[q.Qtype])

  return s.Client.Get(strings.Join(path, "/"))
}

func (s HelixServer) Handler(w dns.ResponseWriter, req *dns.Msg) {
  m := new(dns.Msg)
  m.SetReply(req)

  qType  := req.Question[0].Qtype
  qClass := req.Question[0].Qclass

  header := dns.RR_Header{Name: m.Question[0].Name, Rrtype: qType, Class: qClass, Ttl: 5}

  resp, err := s.getResponse(req.Question[0])

  if err != nil {
    log.Printf("Could not get record for %s", req.Question[0].Name)
    w.WriteMsg(m)
    return
  }

  switch qType {
    case dns.TypeA:
      m.Answer = make([]dns.RR, 1)
      m.Answer[0] = &dns.A {Hdr: header, A: net.ParseIP(resp.Value())}
    case dns.TypeAAAA:
      m.Answer = make([]dns.RR, 1)
      m.Answer[0] = &dns.AAAA {Hdr: header, AAAA: net.ParseIP(resp.Value())}
    case dns.TypeSRV:
      var records []SrvRecord
      err := json.Unmarshal([]byte(resp.Value()), &records)
      if err != nil {
        log.Printf("Could not unmarshal SRV record from etcd: %s", resp.Value())
      } else {
        m.Answer = make([]dns.RR, len(records))
        for i := range records {
          m.Answer[i] = &dns.SRV {
            Hdr:      header,
            Priority: records[i].Priority,
            Weight:   records[i].Weight,
            Port:     records[i].Port,
            Target:   records[i].Target,
          }
        }
      }
    default:
      log.Printf("Unrecognised record type: %d",qType)
  }

  w.WriteMsg(m)
}
