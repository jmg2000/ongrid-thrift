// Autogenerated by Thrift Compiler (0.10.0)
// DO NOT EDIT UNLESS YOU ARE SURE THAT YOU KNOW WHAT YOU ARE DOING

package main

import (
        "flag"
        "fmt"
        "math"
        "net"
        "net/url"
        "os"
        "strconv"
        "strings"
        "git.apache.org/thrift.git/lib/go/thrift"
        "ongrid2"
)


func Usage() {
  fmt.Fprintln(os.Stderr, "Usage of ", os.Args[0], " [-h host:port] [-u url] [-f[ramed]] function [arg1 [arg2...]]:")
  flag.PrintDefaults()
  fmt.Fprintln(os.Stderr, "\nFunctions:")
  fmt.Fprintln(os.Stderr, "  string connect(string macaddr)")
  fmt.Fprintln(os.Stderr, "  void disconnect(string authToken)")
  fmt.Fprintln(os.Stderr, "  string addWorkPlace(string wpname, string macaddr, string login, string password)")
  fmt.Fprintln(os.Stderr, "   getEvents(string authToken, string last)")
  fmt.Fprintln(os.Stderr, "  string postEvent(string authToken, Event event)")
  fmt.Fprintln(os.Stderr, "  CentrifugoConf getCentrifugoConf(string authToken)")
  fmt.Fprintln(os.Stderr, "  ConfigObject getConfiguration(string authToken)")
  fmt.Fprintln(os.Stderr, "   getProps(string authToken)")
  fmt.Fprintln(os.Stderr, "  i64 login(string login, string password)")
  fmt.Fprintln(os.Stderr, "   getUserPrivileges(string authToken, i64 userId)")
  fmt.Fprintln(os.Stderr, "   getUsers(string authToken)")
  fmt.Fprintln(os.Stderr, "  string registerCustomer(string email, string name, string phone)")
  fmt.Fprintln(os.Stderr, "  void ping()")
  fmt.Fprintln(os.Stderr)
  os.Exit(0)
}

func main() {
  flag.Usage = Usage
  var host string
  var port int
  var protocol string
  var urlString string
  var framed bool
  var useHttp bool
  var parsedUrl url.URL
  var trans thrift.TTransport
  _ = strconv.Atoi
  _ = math.Abs
  flag.Usage = Usage
  flag.StringVar(&host, "h", "localhost", "Specify host and port")
  flag.IntVar(&port, "p", 9090, "Specify port")
  flag.StringVar(&protocol, "P", "binary", "Specify the protocol (binary, compact, simplejson, json)")
  flag.StringVar(&urlString, "u", "", "Specify the url")
  flag.BoolVar(&framed, "framed", false, "Use framed transport")
  flag.BoolVar(&useHttp, "http", false, "Use http")
  flag.Parse()
  
  if len(urlString) > 0 {
    parsedUrl, err := url.Parse(urlString)
    if err != nil {
      fmt.Fprintln(os.Stderr, "Error parsing URL: ", err)
      flag.Usage()
    }
    host = parsedUrl.Host
    useHttp = len(parsedUrl.Scheme) <= 0 || parsedUrl.Scheme == "http"
  } else if useHttp {
    _, err := url.Parse(fmt.Sprint("http://", host, ":", port))
    if err != nil {
      fmt.Fprintln(os.Stderr, "Error parsing URL: ", err)
      flag.Usage()
    }
  }
  
  cmd := flag.Arg(0)
  var err error
  if useHttp {
    trans, err = thrift.NewTHttpClient(parsedUrl.String())
  } else {
    portStr := fmt.Sprint(port)
    if strings.Contains(host, ":") {
           host, portStr, err = net.SplitHostPort(host)
           if err != nil {
                   fmt.Fprintln(os.Stderr, "error with host:", err)
                   os.Exit(1)
           }
    }
    trans, err = thrift.NewTSocket(net.JoinHostPort(host, portStr))
    if err != nil {
      fmt.Fprintln(os.Stderr, "error resolving address:", err)
      os.Exit(1)
    }
    if framed {
      trans = thrift.NewTFramedTransport(trans)
    }
  }
  if err != nil {
    fmt.Fprintln(os.Stderr, "Error creating transport", err)
    os.Exit(1)
  }
  defer trans.Close()
  var protocolFactory thrift.TProtocolFactory
  switch protocol {
  case "compact":
    protocolFactory = thrift.NewTCompactProtocolFactory()
    break
  case "simplejson":
    protocolFactory = thrift.NewTSimpleJSONProtocolFactory()
    break
  case "json":
    protocolFactory = thrift.NewTJSONProtocolFactory()
    break
  case "binary", "":
    protocolFactory = thrift.NewTBinaryProtocolFactoryDefault()
    break
  default:
    fmt.Fprintln(os.Stderr, "Invalid protocol specified: ", protocol)
    Usage()
    os.Exit(1)
  }
  client := ongrid2.NewOngridClientFactory(trans, protocolFactory)
  if err := trans.Open(); err != nil {
    fmt.Fprintln(os.Stderr, "Error opening socket to ", host, ":", port, " ", err)
    os.Exit(1)
  }
  
  switch cmd {
  case "connect":
    if flag.NArg() - 1 != 1 {
      fmt.Fprintln(os.Stderr, "Connect requires 1 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    fmt.Print(client.Connect(value0))
    fmt.Print("\n")
    break
  case "disconnect":
    if flag.NArg() - 1 != 1 {
      fmt.Fprintln(os.Stderr, "Disconnect requires 1 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    fmt.Print(client.Disconnect(value0))
    fmt.Print("\n")
    break
  case "addWorkPlace":
    if flag.NArg() - 1 != 4 {
      fmt.Fprintln(os.Stderr, "AddWorkPlace requires 4 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    argvalue2 := flag.Arg(3)
    value2 := argvalue2
    argvalue3 := flag.Arg(4)
    value3 := argvalue3
    fmt.Print(client.AddWorkPlace(value0, value1, value2, value3))
    fmt.Print("\n")
    break
  case "getEvents":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "GetEvents requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    fmt.Print(client.GetEvents(value0, value1))
    fmt.Print("\n")
    break
  case "postEvent":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "PostEvent requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    arg120 := flag.Arg(2)
    mbTrans121 := thrift.NewTMemoryBufferLen(len(arg120))
    defer mbTrans121.Close()
    _, err122 := mbTrans121.WriteString(arg120)
    if err122 != nil {
      Usage()
      return
    }
    factory123 := thrift.NewTSimpleJSONProtocolFactory()
    jsProt124 := factory123.GetProtocol(mbTrans121)
    argvalue1 := ongrid2.NewEvent()
    err125 := argvalue1.Read(jsProt124)
    if err125 != nil {
      Usage()
      return
    }
    value1 := argvalue1
    fmt.Print(client.PostEvent(value0, value1))
    fmt.Print("\n")
    break
  case "getCentrifugoConf":
    if flag.NArg() - 1 != 1 {
      fmt.Fprintln(os.Stderr, "GetCentrifugoConf requires 1 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    fmt.Print(client.GetCentrifugoConf(value0))
    fmt.Print("\n")
    break
  case "getConfiguration":
    if flag.NArg() - 1 != 1 {
      fmt.Fprintln(os.Stderr, "GetConfiguration requires 1 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    fmt.Print(client.GetConfiguration(value0))
    fmt.Print("\n")
    break
  case "getProps":
    if flag.NArg() - 1 != 1 {
      fmt.Fprintln(os.Stderr, "GetProps requires 1 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    fmt.Print(client.GetProps(value0))
    fmt.Print("\n")
    break
  case "login":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "Login requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    fmt.Print(client.Login(value0, value1))
    fmt.Print("\n")
    break
  case "getUserPrivileges":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "GetUserPrivileges requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1, err132 := (strconv.ParseInt(flag.Arg(2), 10, 64))
    if err132 != nil {
      Usage()
      return
    }
    value1 := argvalue1
    fmt.Print(client.GetUserPrivileges(value0, value1))
    fmt.Print("\n")
    break
  case "getUsers":
    if flag.NArg() - 1 != 1 {
      fmt.Fprintln(os.Stderr, "GetUsers requires 1 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    fmt.Print(client.GetUsers(value0))
    fmt.Print("\n")
    break
  case "registerCustomer":
    if flag.NArg() - 1 != 3 {
      fmt.Fprintln(os.Stderr, "RegisterCustomer requires 3 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    argvalue2 := flag.Arg(3)
    value2 := argvalue2
    fmt.Print(client.RegisterCustomer(value0, value1, value2))
    fmt.Print("\n")
    break
  case "ping":
    if flag.NArg() - 1 != 0 {
      fmt.Fprintln(os.Stderr, "Ping requires 0 args")
      flag.Usage()
    }
    fmt.Print(client.Ping())
    fmt.Print("\n")
    break
  case "":
    Usage()
    break
  default:
    fmt.Fprintln(os.Stderr, "Invalid function ", cmd)
  }
}
