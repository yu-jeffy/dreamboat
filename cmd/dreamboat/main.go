package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"

	"time"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/pkg/relay"
	"github.com/blocknative/dreamboat/pkg/service"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/sirupsen/logrus"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	blst "github.com/supranational/blst/bindings/go"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/blocknative/dreamboat/pkg/store/datastore"
)

const (
	shutdownTimeout = 5 * time.Second
	version         = relay.Version
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
	svr http.Server
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
		cfg.Log.Fatal(err)
	}
}

func setup() cli.BeforeFunc {
	return func(c *cli.Context) (err error) {
		sk, pk, err := setupKeys(c)
		if err != nil {
			return err
		}

		cfg = config.Config{
			Log:                 config.Logger(c),
			RelayRequestTimeout: c.Duration("timeout"),
			Network:             c.String("network"),
			BuilderCheck:        c.Bool("check-builder"),
			BuilderURLs:         c.StringSlice("builder"),
			BeaconEndpoints:     c.StringSlice("beacon"),
			PubKey:              pk,
			SecretKey:           sk,
			Datadir:             c.String("datadir"),
			TTL:                 c.Duration("ttl"),
			CheckKnownValidator: c.Bool("checkKnownValidator"),
		}

		svr = http.Server{
			Addr:         c.String("addr"),
			ReadTimeout:  c.Duration("timeout"),
			WriteTimeout: c.Duration("timeout"),
			IdleTimeout:  time.Second * 2,

			MaxHeaderBytes: 4096,
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

		// setup the relay service
		service := &service.DefaultService{
			Log: cfg.Log,
			TTL: cfg.TTL,
		}

		if s.Log == nil {
			s.Log = log.New().WithField("service", "RelayService")
		}

		timeRelayStart := time.Now()
		if s.Relay == nil {
			s.Relay, err = relay.NewRelay(s.Config)
			if err != nil {
				return
			}
		}
		s.Log.WithFields(logrus.Fields{
			"service":     "relay",
			"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
		}).Info("initialized")

		timeDataStoreStart := time.Now()
		if s.Datastore == nil {
			if s.Storage == nil {
				storage, err := badger.NewDatastore(s.Config.Datadir, &badger.DefaultOptions)
				if err != nil {
					s.Log.WithError(err).Fatal("failed to initialize datastore")
					return err
				}
				s.Storage = &TTLDatastoreBatcher{storage}
			}

			s.Datastore = &datastore.Datastore{TTLStorage: s.Storage}
		}
		s.Log.
			WithFields(logrus.Fields{
				"service":     "datastore",
				"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
			}).Info("data store initialized")

		s.state.datastore.Store(s.Datastore)

		if s.NewBeaconClient == nil {
			s.NewBeaconClient = func() (BeaconClient, error) {
				clients := make([]BeaconClient, 0, len(s.Config.BeaconEndpoints))
				for _, endpoint := range s.Config.BeaconEndpoints {
					client, err := NewBeaconClient(endpoint, s.Config)
					if err != nil {
						return nil, err
					}
					clients = append(clients, client)
				}
				return NewMultiBeaconClient(s.Config.Log.WithField("service", "multi-beacon client"), clients), nil
			}
		}

		g.Go(func() error {

			client, err := s.NewBeaconClient()
			if err != nil {
				s.Log.WithError(err).Warn("failed beacon client registration")
				return err
			}

			s.Log.Info("beacon client initialized")

			return s.beaconEventLoop(ctx, client)

			//return service.Run(ctx)
		})

		// wait for the relay service to be ready
		select {
		case <-service.Ready():
		case <-ctx.Done():
			return g.Wait()
		}

		cfg.Log.Debug("relay service ready")

		// run the http server
		g.Go(func() (err error) {
			svr.BaseContext = func(l net.Listener) context.Context {
				return ctx
			}

			svr.Handler = &api.API{
				Service:       service,
				Log:           cfg.Log,
				EnableProfile: c.Bool("profile"),
			}

			cfg.Log.Info("http server listening")
			if err = svr.ListenAndServe(); err == http.ErrServerClosed {
				err = nil
			}

			return err
		})

		g.Go(func() error {
			defer svr.Close()
			<-ctx.Done()

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()

			return svr.Shutdown(ctx)
		})

		return g.Wait()
	}
}
