package main

import (
	"context"
	"errors"

	"net"
	"net/http"
	"os"

	"time"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/blocknative/dreamboat/pkg/beacon"
	beaconCli "github.com/blocknative/dreamboat/pkg/client/beacon"
	"github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/store/datastore"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"

	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	blst "github.com/supranational/blst/bindings/go"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
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
		Before:  setup(),
		Action:  run(),
	}

	if err := app.Run(os.Args); err != nil {
		l.Fatal(err)
	}
}

func setup() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		sk, pk, err := setupKeys(c)
		if err != nil {
			return err
		}

		cfg = config.Config{
			RelayRequestTimeout: c.Duration("timeout"),
			Network:             c.String("network"),
			BuilderCheck:        c.Bool("check-builder"),
			BuilderURLs:         c.StringSlice("builder"),
			BeaconEndpoints:     c.StringSlice("beacon"),
			PubKey:              pk,
			SecretKey:           sk,
			Datadir:             c.String("datadir"),
			CheckKnownValidator: c.Bool("checkKnownValidator"),
		}

		return
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

func run() cli.ActionFunc {
	return func(c *cli.Context) error {
		g, ctx := errgroup.WithContext(c.Context)

		l := log.New() //.WithField("service", "RelayService")

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

		multiBClient := beacon.NewMultiBeaconClient(l.WithField("service", "multi-beacon client"), clients)
		/*if err != nil {
			l.WithError(err).Warn("failed beacon client registration")
			return err
		}*/

		l.Info("beacon client initialized")

		bm := beacon.NewBeaconManager(l, multiBClient)
		go bm.BeaconEventLoop(ctx)

		// RELAY INITIALIZATION
		if err := cfg.Validate(); err != nil {
			return err
		}

		domainBuilder, err := ComputeDomain(types.DomainTypeAppBuilder, cfg.GenesisForkVersion, types.Root{}.String())
		if err != nil {
			return err
		}

		domainBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, cfg.BellatrixForkVersion, cfg.GenesisValidatorsRoot)
		if err != nil {
			return err
		}
		r, err := relay.NewRelay(cfg, l, store, bm, domainBuilder, domainBeaconProposer)
		r.TTL = c.Duration("ttl")
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

		api := api.NewApi(l, r)
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
			BaseContext: func(l net.Listener) context.Context { // TODO(l): remove this
				return ctx
			},
		}

		g.Go(func() error {
			defer svr.Close()
			<-ctx.Done()

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			return svr.Shutdown(ctx)
		})

		l.Info("http server listening")
		return svr.ListenAndServe()
	}
}

// ComputeDomain computes the signing domain
func ComputeDomain(domainType types.DomainType, forkVersionHex string, genesisValidatorsRootHex string) (domain types.Domain, err error) {
	genesisValidatorsRoot := types.Root(common.HexToHash(genesisValidatorsRootHex))
	forkVersionBytes, err := hexutil.Decode(forkVersionHex)
	if err != nil || len(forkVersionBytes) > 4 {
		err = errors.New("invalid fork version passed")
		return domain, err
	}
	var forkVersion [4]byte
	copy(forkVersion[:], forkVersionBytes[:4])
	return types.ComputeDomain(domainType, forkVersion, genesisValidatorsRoot), nil
}
