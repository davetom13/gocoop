package internal

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/fallais/gocoop/internal/cache"
	"github.com/fallais/gocoop/internal/routes"
	"github.com/fallais/gocoop/internal/services"
	"github.com/fallais/gocoop/internal/system/middleware"
	"github.com/fallais/gocoop/pkg/coop"
	"github.com/fallais/gocoop/pkg/coop/conditions/sunbased"
	"github.com/fallais/gocoop/pkg/coop/conditions/timebased"
	"github.com/fallais/gocoop/pkg/door"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"goji.io"
	"goji.io/pat"
)

// Run is a convenient function for Cobra.
func Run(cmd *cobra.Command, args []string) {
	// Flags
	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		logrus.WithError(err).Fatalln("Error while getting the flag for configuration data")
	}

	// Read configuration file
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		logrus.WithError(err).Fatalln("Error while reading configuration file")
	}

	// Initialize configuration values with Viper
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(data))
	if err != nil {
		logrus.WithError(err).Fatalln("Error when reading configuration data")
	}

	opts := coop.Options{
		Latitude:    viper.GetFloat64("coop.latitude"),
		Longitude:   viper.GetFloat64("coop.longitude"),
		IsAutomatic: viper.GetBool("coop.is_automatic"),
	}

	// Create the opening condition
	logrus.WithFields(logrus.Fields{
		"mode":  viper.GetString("coop.opening.mode"),
		"value": viper.GetString("coop.opening.value"),
	}).Infoln("Creating the opening condition")
	switch viper.GetString("coop.opening.mode") {
	case "time_based":
		openingCondition, err := timebased.NewTimeBasedCondition(viper.GetString("coop.opening.value"))
		if err != nil {
			logrus.WithError(err).Fatalln("Error while creating the opening condition")
		}

		opts.OpeningCondition = openingCondition
	case "sun_based":
		openingCondition, err := sunbased.NewSunBasedCondition(viper.GetString("coop.opening.value"), viper.GetFloat64("coop.latitude"), viper.GetFloat64("coop.longitude"))
		if err != nil {
			logrus.WithError(err).Fatalln("Error while creating the opening condition")
		}

		opts.OpeningCondition = openingCondition
	default:
		logrus.WithError(err).Fatalf("error with the opening mode: %s", viper.GetString("coop.opening.mode"))
	}
	logrus.WithFields(logrus.Fields{
		"mode":  viper.GetString("coop.opening.mode"),
		"value": viper.GetString("coop.opening.value"),
	}).Infoln("Successfully created the opening condition")

	// Create the closing condition
	logrus.WithFields(logrus.Fields{
		"mode":  viper.GetString("coop.closing.mode"),
		"value": viper.GetString("coop.closing.value"),
	}).Infoln("Creating the closing condition")
	switch viper.GetString("coop.closing.mode") {
	case "time_based":
		closingCondition, err := timebased.NewTimeBasedCondition(viper.GetString("coop.closing.value"))
		if err != nil {
			logrus.WithError(err).Fatalln("Error when creating the closing condition")
		}

		opts.ClosingCondition = closingCondition
	case "sun_based":
		closingCondition, err := sunbased.NewSunBasedCondition(viper.GetString("coop.closing.value"), viper.GetFloat64("coop.latitude"), viper.GetFloat64("coop.longitude"))
		if err != nil {
			logrus.WithError(err).Fatalln("Error when creating the closing condition")
		}

		opts.ClosingCondition = closingCondition
	default:
		logrus.WithError(err).Fatalf("error with the closing mode: %s", viper.GetString("coop.closing.mode"))
	}
	logrus.Infoln("Successfully created the closing condition")

	// Door
	logrus.WithFields(logrus.Fields{
		"pin_1A":      viper.GetString("coop.pin_1A"),
		"pin_1B":      viper.GetString("coop.pin_1B"),
		"pin_enable1": viper.GetString("coop.pin_enable1"),
	}).Infoln("Creating the door")
	opts.Door = door.NewDoor(viper.GetInt("door.pin_1A"), viper.GetInt("door.pin_1B"), viper.GetInt("door.pin_enable1"), viper.GetDuration("door.opening_duration"), viper.GetDuration("door.closing_duration"))
	logrus.Infoln("Successfully created the door")

	// Create the coop instance
	c, err := coop.New(opts)
	if err != nil {
		logrus.WithError(err).Fatalln("Error while creating the coop instance")
	}

	// Initialize cache
	logrus.Infoln("Initializing the Redis cache")
	store, err := cache.NewRedisCache(viper.GetString("general.redis_host"), viper.GetString("general.redis_password"), 12*time.Hour)
	if err != nil {
		logrus.WithError(err).Fatalln("Error when initializing connection to Redis cache")
	}
	logrus.Infoln("Successfully initialized the Redis cache")

	// Initialize the middlewares
	logrus.Infoln("Initializing the middlewares")
	corsMiddleware := middleware.Cors()
	jwtMiddleware := middleware.JWT(store, viper.GetString("general.private_key"))
	logrus.Infoln("Successfully initialized the middlewares")

	// Initialize Web controllers
	logrus.Infoln("Initializing the services")
	coopService := services.NewCoopService(c)
	jwtService := services.NewJwtService(store, viper.GetString("general.private_key"))
	logrus.Infoln("Successfully initialized the services")

	// Initialize Web controllers
	logrus.Infoln("Initializing the Web controllers")
	coopCtrl := routes.NewCoopController(coopService)
	miscCtrl := routes.NewMiscController()
	jwtCtrl := routes.NewJwtController(jwtService, viper.GetString("general.gui_username"), viper.GetString("general.gui_password"))
	logrus.Infoln("Successfully initialized the Web controllers")

	// Create a new Goji multiplexer
	root := goji.NewMux()

	// Middlewares
	root.Use(corsMiddleware)

	// Unauthenticated route
	root.HandleFunc(pat.Post("/api/v1"), miscCtrl.Hello)
	root.HandleFunc(pat.Post("/api/v1/login"), jwtCtrl.Login)
	root.HandleFunc(pat.Get("/api/v1/refresh"), jwtCtrl.Refresh)
	root.HandleFunc(pat.Get("/api/v1/logout"), jwtCtrl.Logout)

	// Authenticated routes
	authenticated := goji.SubMux()
	authenticated.Use(jwtMiddleware)
	authenticated.HandleFunc(pat.Get("/api/v1/cameras"), miscCtrl.Cameras)
	authenticated.HandleFunc(pat.Get("/api/v1/coop"), coopCtrl.Get)
	authenticated.HandleFunc(pat.Post("/api/v1/coop"), coopCtrl.Update)
	authenticated.HandleFunc(pat.Post("/api/v1/coop/open"), coopCtrl.Open)
	authenticated.HandleFunc(pat.Post("/api/v1/coop/close"), coopCtrl.Close)

	// Merge the muxes
	root.Handle(pat.New("/*"), authenticated)

	// Handlers
	http.Handle("/", root)

	// Serve
	logrus.Infoln("Starting the Web server")
	err = http.ListenAndServe(":8000", root)
	if err != nil {
		logrus.WithError(err).Fatalln("Error while starting the Web server")
	}
}
