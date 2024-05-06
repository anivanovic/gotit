package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anivanovic/gotit/pkg/logger"
	"github.com/anivanovic/gotit/pkg/printer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type (
	AppConfig struct {
		Log struct {
			Level  string `mapstructure:"level"`
			Format string `mapstructure:"format"`
		} `mapstructure:"log"`
		CfgPath string
	}

	App struct {
		appCfg  *AppConfig
		rootCmd *cobra.Command
	}

	AppContext struct {
		log     *zap.Logger
		printer printer.Printer
	}
)

func NewApp() *App {
	rootCmd := cobra.Command{
		Use: "gotit",
	}
	cobra.OnInitialize()
	cfg := AppConfig{}
	rootCmd.PersistentFlags().StringVarP(&cfg.Log.Level, "log-level", "l", "info", "App logging level [info,debug,trace,warning,error,silent]")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Format, "log-format", "color", "Logging format [text,color,json]")
	rootCmd.PersistentFlags().StringVar(&cfg.CfgPath, "config", "", "Config file location")
	viper.BindPFlag("log.level", rootCmd.Flags().Lookup("log-level"))
	viper.BindPFlag("log.format", rootCmd.Flags().Lookup("log-format"))

	app := &App{
		appCfg:  &cfg,
		rootCmd: &rootCmd,
	}

	rootCmd.AddCommand(NewCommand(app))
	rootCmd.AddCommand(NewDownloadCommand(app))
	rootCmd.AddCommand(NewVersionCommand())

	return app
}

func (a App) Execute() error {
	return a.rootCmd.Execute()
}

func initConfig(cfgPath string) error {
	if cfgPath != "" {
		viper.SetConfigFile(cfgPath)
	} else {
		viper.AddConfigPath("$HOME")
		viper.AddConfigPath(".")
		viper.SetConfigName(".gotit")
	}

	viper.SetEnvPrefix("gotit")
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		var cfgNotFound viper.ConfigFileNotFoundError

		// if we do not find configuration and we specified it
		isPathErr := errors.As(err, &cfgNotFound)
		if isPathErr && cfgPath != "" {
			return cfgNotFound
		}

		if !isPathErr {
			return err
		}
	}

	return nil
}

func (a App) NewCmdRun(
	fn func(ctx context.Context, appCtx AppContext, args []string) error,
) func(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)

	if err := initConfig(a.appCfg.CfgPath); err != nil {
		fmt.Println("could not resolve configuration", err)
		os.Exit(1)
	}

	logCfg := a.appCfg.Log
	l, err := logger.NewCommandLine(logCfg.Level, logCfg.Format)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, "")
	}
	appCtx := AppContext{log: l, printer: printer.NewConsole()}
	return func(_ *cobra.Command, args []string) {
		defer stop()
		if err := fn(ctx, appCtx, args); err != nil {
			appCtx.printer.Fatal(1, "error with the app")
		}
	}
}
