// Command dory-node es el nodo local-first de Dory.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/nodeapp"
	"github.com/migueel15/dory/node/peer"
)

const version = "0.0.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "join" {
		if err := runJoin(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "dory-node join:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "import" {
		if err := runImport(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "dory-node import:", err)
			os.Exit(1)
		}
		return
	}
	if err := runServe(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dory-node:", err)
		os.Exit(1)
	}
}

func defaultDataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "dory-node")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "dory-node")
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("dory-node", flag.ContinueOnError)
	var o nodeapp.Options
	fs.StringVar(&o.DataDir, "data-dir", defaultDataDir(), "directory with the identity, database and group")
	fs.StringVar(&o.Listen, "listen", "127.0.0.1:8080", "API, MCP and SPA address (localhost only)")
	fs.IntVar(&o.PeerPort, "peer-port", 7710, "mTLS port for other devices")
	fs.BoolVar(&o.Headless, "headless", false, "do not serve the SPA")
	fs.BoolVar(&o.NoMDNS, "no-mdns", false, "disable mDNS announce and browse")
	fs.StringVar(&o.T3URL, "t3-url", "", "local T3 URL (default http://localhost:3773 when $DORY_T3_TOKEN is set)")
	fs.StringVar(&o.DeviceName, "name", "", "device name for a new identity (default hostname)")
	pair := fs.Bool("pair", false, "open a pairing window and print the QR payload (and pair.png)")
	showVersion := fs.Bool("version", false, "print the version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println("dory-node", version)
		return nil
	}
	if o.DeviceName == "" {
		o.DeviceName, _ = os.Hostname()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	o.Confirm = confirmOnTerminal
	n, err := nodeapp.Start(ctx, o)
	if err != nil {
		return err
	}
	defer n.Stop()
	log.Printf("device %s (%s), data in %s", n.Identity.DeviceID(), n.Identity.Name, o.DataDir)
	log.Printf("API/MCP: http://%s (token file %s)", n.HTTPAddr, n.TokenFile)
	if !o.Headless {
		log.Printf("UI: open http://%s (signs in as the local user automatically)", n.HTTPAddr)
	}
	log.Printf("peer port: %s (mTLS, members only)", n.PeerAddr)
	if *pair {
		offer, err := n.Pairing.Start(0)
		if err != nil {
			return err
		}
		log.Printf("pairing open until %s. On the other device run: dory-node join '<payload>'\n%s", offer.Expires.Format("15:04:05"), offer.Payload)
		if png, err := offer.PNG(320); err == nil {
			p := filepath.Join(o.DataDir, "pair.png")
			if os.WriteFile(p, png, 0o600) == nil {
				log.Printf("QR image: %s", p)
			}
		}
	}
	<-ctx.Done()
	return nil
}

// confirmOnTerminal asks on stdin whether to accept a pairing device.
func confirmOnTerminal(name, pub string) bool {
	fmt.Printf("Accept device %q (key %.16s...)? [y/N] ", name, pub)
	var ans string
	fmt.Scanln(&ans)
	return strings.EqualFold(ans, "y")
}

// runJoin pairs this (not yet started) node with the one that shows the QR,
// adopting its group and data. Run it before the first start.
func runJoin(args []string) error {
	fs := flag.NewFlagSet("join", flag.ContinueOnError)
	dir := fs.String("data-dir", defaultDataDir(), "directory with the identity, database and group")
	name, _ := os.Hostname()
	fs.StringVar(&name, "name", name, "name of this device")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: dory-node join [--data-dir DIR] [--name NAME] PAYLOAD")
	}
	if _, err := os.Stat(filepath.Join(*dir, "group.json")); err == nil {
		return fmt.Errorf("%s already belongs to a group", *dir)
	}
	id, err := peer.LoadOrCreate(*dir, name)
	if err != nil {
		return err
	}
	store, err := localstore.Open(filepath.Join(*dir, "dory-node.db"), localstore.Options{Device: id.DeviceID(), Local: true})
	if err != nil {
		return err
	}
	defer store.Close()
	ctx := context.Background()
	res, err := peer.Join(ctx, fs.Arg(0), id, name)
	if err != nil {
		return err
	}
	if err = peer.SaveGroup(*dir, res.Group); err != nil {
		return err
	}
	if err = res.ApplySnapshot(ctx, store.Log()); err != nil {
		return err
	}
	fmt.Printf("joined the group of %s\n", res.HostAddr)
	return nil
}
