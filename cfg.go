package main

import (
	"log"
	"time"

	"github.com/spf13/viper"
)

func cfgNew(cfgFile string) (*viper.Viper, error) {
	vpr := viper.New()

	// init default values
	vpr.SetDefault("address", "54321")
	vpr.SetDefault("dburl", "postgres://user:password@localhost:5432/dbname")
	vpr.SetDefault("sqlroot", "/path/to/sql/files")
	vpr.SetDefault("keepalive", false)
	vpr.SetDefault("querytimeout", (time.Second * 60))
	vpr.SetDefault("autotest", false)
	vpr.SetDefault("testworkers", 1)
	vpr.SetDefault("ignorerrors", false)
	vpr.SetDefault("mutedberrors", true)
	vpr.SetDefault("usetls", true)
	vpr.SetDefault("certfile", "certfile.crt")
	vpr.SetDefault("keyfile", "keyfile.key")
	vpr.SetDefault("filteroutparams", true)
	vpr.SetDefault("filterinparams", true)
	vpr.SetDefault("loginrequired", true)
	vpr.SetDefault("sessionlifetime", (time.Second * 300))
	vpr.SetDefault("loginquery", "/login")
	vpr.SetDefault("cookiesession", true)
	vpr.SetDefault("logoutquery", "")
	vpr.SetDefault("docenable", true)

	// load from file
	log.Printf("Load config from %s\n", cfgFile)
	vpr.SetConfigType("toml")
	vpr.SetConfigFile(cfgFile)

	if err := vpr.ReadInConfig(); err != nil {
		return nil, err
	}

	return vpr, nil
}

func cfgPrint(cfg *viper.Viper) {
	log.Println("---CONFIG---")
	for key, val := range cfg.AllSettings() {
		log.Println(key, " = ", val)
	}
	log.Println("---CONFIG END---")
}
