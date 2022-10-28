package main

import (
	"context"

	"net/http"
	"os"

	"time"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/blocknative/dreamboat/pkg/beacon"
	beaconCli "github.com/blocknative/dreamboat/pkg/client/beacon"
	"github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/store/beacon/memstore"
	"github.com/blocknative/dreamboat/pkg/store/datastore"
	"github.com/blocknative/dreamboat/pkg/structs"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"

	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	blst "github.com/supranational/blst/bindings/go"
	"github.com/urfave/cli/v2"
)

const (
	shutdownTimeout = 5 * time.Second
	version         = "0.2.0"
)

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "loglvl",
		Usage:   "logging level: trace, debug, info, warn, error or fatal",
		Value:   "info",
		EnvVars: []string{"LOGLVL"},
	},
	&cli.StringFlag{
		Name:    "logfmt",
		Usage:   "format logs as text, json or none",
		Value:   "text",
		EnvVars: []string{"LOGFMT"},
	},
	&cli.BoolFlag{
		Name:  "profile",
		Usage: "activates profiling http endpoint",
		Value: false,
	},
	&cli.StringFlag{
		Name:    "addr",
		Usage:   "server listen address",
		Value:   "localhost:18550",
		EnvVars: []string{"RELAY_ADDR"},
	},
	&cli.DurationFlag{
		Name:    "timeout",
		Usage:   "request timeout",
		Value:   time.Second * 2,
		EnvVars: []string{"RELAY_TIMEOUT"},
	},
	&cli.StringSliceFlag{
		Name:    "beacon",
		Usage:   "`url` for beacon endpoint",
		EnvVars: []string{"RELAY_BEACON"},
	},
	&cli.BoolFlag{
		Name:    "check-builders",
		Usage:   "check builder blocks",
		EnvVars: []string{"RELAY_CHECK_BUILDERS"},
	},
	&cli.StringSliceFlag{
		Name:    "builder",
		Usage:   "`url` formatted as schema://pubkey@host",
		EnvVars: []string{"BN_RELAY_BUILDER_URLS"},
	},
	&cli.StringFlag{
		Name:    "network",
		Usage:   "the networks the relay works on",
		Value:   "mainnet",
		EnvVars: []string{"RELAY_NETWORK"},
	},
	&cli.StringFlag{
		Name:     "secretKey",
		Usage:    "secret key used to sign messages",
		Required: true,
		EnvVars:  []string{"RELAY_SECRET_KEY"},
	},
	&cli.StringFlag{
		Name:    "datadir",
		Usage:   "data directory where blocks and validators are stored in the default datastore implementation",
		Value:   "/tmp/relay",
		EnvVars: []string{"RELAY_DATADIR"},
	},
	&cli.DurationFlag{
		Name:    "ttl",
		Usage:   "ttl of the data",
		Value:   24 * time.Hour,
		EnvVars: []string{"BN_RELAY_TTL"},
	},
	&cli.BoolFlag{
		Name:  "checkKnownValidator",
		Usage: "rejects validator registration if it's not a known validator from the beacon",
		Value: false,
	},
}

var (
	cfg config.Config
)

func init() {
	cfg = config.NewConfig()

}

// Main starts the relay
func main() {
	app := &cli.App{
		Name:    "dreamboat",
		Usage:   "ethereum 2.0 relay, commissioned and put to sea by Blocknative",
		Version: version,
		Flags:   flags,
		Action:  run(),
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		//g, ctx := errgroup.WithContext(c.Context)
		ctx := context.Background()

		cfg = config.Config{
			RelayRequestTimeout: c.Duration("timeout"),
			Network:             c.String("network"),
			BuilderCheck:        c.Bool("check-builder"),
			BuilderURLs:         c.StringSlice("builder"),
			BeaconEndpoints:     c.StringSlice("beacon"),
			Datadir:             c.String("datadir"),
			CheckKnownValidator: c.Bool("checkKnownValidator"),
		}

		l := log.New()

		// DATASTORE INITIALIZATION
		storage, err := badger.NewDatastore(cfg.Datadir, &badger.DefaultOptions)
		if err != nil {
			l.WithError(err).Fatal("failed to initialize datastore")
			return err
		}
		store := datastore.NewDatastore(storage)
		defer store.Close()

		// BEACON MANAGER INITIALIZATION
		clients := make([]*beaconCli.BeaconClient, 0, len(cfg.BeaconEndpoints))
		for _, endpoint := range cfg.BeaconEndpoints {
			client, err := beaconCli.NewBeaconClient(endpoint, l)
			if err != nil {
				return err
			}
			clients = append(clients, client)
		}

		memoryStore := memstore.NewBeaconMemstore()
		multiBClient := beacon.NewMultiBeaconClient(l.WithField("service", "multi-beacon client"), clients)

		l.Info("beacon client initialized")

		bm := beacon.NewBeaconManager(l, memoryStore, multiBClient)
		go bm.BeaconEventLoop(ctx)

		// RELAY INITIALIZATION
		if err := cfg.Validate(); err != nil {
			return err
		}

		domainBuilder, err := structs.ComputeDomain(types.DomainTypeAppBuilder, cfg.GenesisForkVersion, types.Root{}.String())
		if err != nil {
			return err
		}

		domainBeaconProposer, err := structs.ComputeDomain(types.DomainTypeBeaconProposer, cfg.BellatrixForkVersion, cfg.GenesisValidatorsRoot)
		if err != nil {
			return err
		}

		sk, pk, err := setupKeys(c)
		if err != nil {
			return err
		}

		rcfg := relay.RelayConfig{
			TTL:       c.Duration("ttl"),
			PubKey:    pk,
			SecretKey: sk,
		}

		r, err := relay.NewRelay(rcfg, l, store, domainBuilder, domainBeaconProposer)

		if err != nil {
			return err
		}
		/*
			l.WithFields(logrus.Fields{
				"service":     "relay",
				"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
			}).Info("initialized")

			timeDataStoreStart := time.Now()

			l.
				WithFields(logrus.Fields{
					"service":     "datastore",
					"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
				}).Info("data store initialized")
		*/

		api := api.NewApi(l, memoryStore, r, cfg.CheckKnownValidator)
		mux := http.NewServeMux()
		api.AttachToHandler(mux)
		l.Debug("relay service ready")

		svr := http.Server{
			Addr:           c.String("addr"),
			ReadTimeout:    c.Duration("timeout"),
			WriteTimeout:   c.Duration("timeout"),
			IdleTimeout:    time.Second * 2,
			MaxHeaderBytes: 4096,
			Handler:        mux,
		}

		var i int
		for {
			i++
			if memoryStore.UpdateTime().Second() > 0 {
				break
			}
			if i > 200 {
				// FATAL ERROR
			}
			<-time.After(time.Second)
		}

		go func(ctx context.Context, srv *http.Server) {
			defer svr.Close()
			<-ctx.Done()

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			srv.Shutdown(ctx)
		}(ctx, &svr)

		l.Info("http server listening")
		return svr.ListenAndServe()
	}
}

func setupKeys(c *cli.Context) (*blst.SecretKey, types.PublicKey, error) {
	skBytes, err := hexutil.Decode(c.String("secretKey"))
	if err != nil {
		return nil, types.PublicKey{}, err
	}
	sk, err := bls.SecretKeyFromBytes(skBytes[:])
	if err != nil {
		return nil, types.PublicKey{}, err
	}

	var pk types.PublicKey
	err = pk.FromSlice(bls.PublicKeyFromSecretKey(sk).Compress())
	return sk, pk, err
}
