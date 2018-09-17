package main

import (
	"github.com/urfave/cli"
)

var (
	appFlags []cli.Flag
)

func init() {
	appFlags = []cli.Flag{
		cli.StringFlag{
			Name: "listen,l", Value: ":29900", Usage: "kcp server listen address",
		},
		cli.StringFlag{
			Name: "target, t", Value: "127.0.0.1:12948", Usage: "target server address",
		},
		cli.StringFlag{
			Name: "key", Value: "it's a secrect", EnvVar: "KCPTUN_KEY", Usage: "pre-shared secret between client and server",
		},
		cli.StringFlag{
			Name: "crypt", Value: "aes", Usage: "aes, aes-128, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, sm4, none",
		},
		cli.StringFlag{
			Name: "mode", Value: "fast", Usage: "profiles: fast3, fast2, fast, normal, manual",
		},
		cli.IntFlag{
			Name: "mtu", Value: 1350, Usage: "set maximum transmission unit for UDP packets",
		},
		cli.IntFlag{
			Name: "sndwnd", Value: 1024, Usage: "set send window size(num of packets)",
		},
		cli.IntFlag{
			Name: "rcvwnd", Value: 1024, Usage: "set receive window size(num of packets)",
		},
		cli.IntFlag{
			Name: "datashard,ds", Value: 10, Usage: "set reed-solomon erasure coding - datashard",
		},
		cli.IntFlag{
			Name: "parityshard,ps", Value: 3, Usage: "set reed-solomon erasure coding - parityshard",
		},
		cli.IntFlag{
			Name: "dscp", Value: 0, Usage: "set DSCP(6bit)",
		},
		cli.BoolFlag{
			Name: "nocomp", Usage: "disable compression",
		},
		cli.BoolFlag{
			Name: "acknodelay", Hidden: true, Usage: "flush ack immediately when a packet is received",
		},
		cli.IntFlag{
			Name: "nodelay", Value: 0, Hidden: true,
		},
		cli.IntFlag{
			Name: "interval", Value: 50, Hidden: true,
		},
		cli.IntFlag{
			Name: "resend", Value: 0, Hidden: true,
		},
		cli.IntFlag{
			Name: "nc", Value: 0, Hidden: true,
		},
		cli.IntFlag{
			Name: "sockbuf", Value: 4194304, Hidden: true,
		},
		cli.IntFlag{
			Name: "keepalive", Value: 10, Hidden: true,
		},
		cli.StringFlag{
			Name: "snmplog", Value: "", Usage: "collect snmp to file, aware of timeformat in golang, like: ./snmp-20060102.log",
		},
		cli.IntFlag{
			Name: "snmpperiod", Value: 60, Usage: "snmp collect period, in seconds",
		},
		cli.BoolFlag{
			Name: "pprof", Usage: "start profiling server on :6060",
		},
		cli.StringFlag{
			Name: "log", Value: "", Usage: "specify a log file to output, default goes to stderr",
		},
		cli.BoolFlag{
			Name: "quiet", Usage: "to suppress the 'stream open/close' messages",
		},
		cli.StringFlag{
			Name: "c", Value: "", Usage: "config from json file, which will override the command from shell",
		},
	}
}
