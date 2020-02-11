package coop

import (
	"errors"
	"fmt"
	"gocoop/internal/protocols"
	"time"

	"gocoop/pkg/coop/conditions"
	"gocoop/pkg/door"

	"github.com/sirupsen/logrus"
)

// ErrAutomaticModeEnabled is raised when the automatic mode is enabled.
var ErrAutomaticModeEnabled = errors.New("cannot close the coop because automatic mode is enabled")

// ErrCoopAlreadyOpening ...
var ErrCoopAlreadyOpening = errors.New("coop is already opening")

// ErrCoopAlreadyClosing ...
var ErrCoopAlreadyClosing = errors.New("coop is already closing")

// CheckFrequency is the frequency for checking the coop.
const CheckFrequency = 10 * time.Second

// DefaultLatitude is the default latitude.
const DefaultLatitude = 43.6043

// DefaultLongitude is the default longitude.
const DefaultLongitude = 1.4437

//------------------------------------------------------------------------------
// Structure
//------------------------------------------------------------------------------

// Coop represents a chicken coop.
type Coop struct {
	door      *door.Door
	opts      options
	status    Status
	latitude  float64
	longitude float64
	ticker    *time.Ticker
}

//------------------------------------------------------------------------------
// Factory
//------------------------------------------------------------------------------

// New returns a new Coop.
func New(latitude, longitude float64, door *door.Door, opts ...Option) (*Coop, error) {
	c := &Coop{
		latitude:  latitude,
		longitude: longitude,
		ticker:    time.NewTicker(10 * time.Second),
	}

	for _, opt := range opts {
		opt(&c.opts)
	}

	// Check latitude
	if latitude == 0 {
		c.latitude = DefaultLatitude
	}

	// Check longitude
	if longitude == 0 {
		c.longitude = DefaultLongitude
	}

	// Watch the clock
	go c.watch()

	// Notify that the status is unknown
	go c.notify()

	return c, nil

}

//------------------------------------------------------------------------------
// Functions
//------------------------------------------------------------------------------

func (coop *Coop) watch() {
	for range coop.ticker.C {
		go coop.Check()
	}
}

func (coop *Coop) notify() {
	logrus.Infoln("Notifying")
	for _, notifier := range coop.opts.notifiers {
		notifier.Notify("The status of the coop is unknown.")
	}
	logrus.Infoln("Successfully notified")
}

// Status returns the status of the chicken coop.
func (coop *Coop) Status() Status {
	return coop.status
}

// IsAutomatic returns the automatic mode.
func (coop *Coop) IsAutomatic() bool {
	return coop.opts.isAutomatic
}

// Latitude returns the latitude of the chicken coop.
func (coop *Coop) Latitude() float64 {
	return coop.latitude
}

// Longitude returns the longitude of the chicken coop.
func (coop *Coop) Longitude() float64 {
	return coop.longitude
}

// OpeningCondition returns the opening condition of the chicken coop.
func (coop *Coop) OpeningCondition() conditions.Condition {
	return coop.opts.openingCondition
}

// ClosingCondition returns the closing condition of the chicken coop.
func (coop *Coop) ClosingCondition() conditions.Condition {
	return coop.opts.closingCondition
}

// OpeningTime returns the opening time of the chicken coop.
func (coop *Coop) OpeningTime() time.Time {
	return coop.opts.openingCondition.OpeningTime()
}

// ClosingTime returns the opening time of the chicken coop.
func (coop *Coop) ClosingTime() time.Time {
	return coop.opts.closingCondition.ClosingTime()
}

// Update updates the chicken coop.
func (coop *Coop) Update(input protocols.CoopUpdateRequestService) error {
	// Update the status
	switch input.Status {
	case "opened":
		coop.status = Opened
	case "closed":
		coop.status = Closed
	default:
		return fmt.Errorf("status is incorrect")
	}

	// Update the automatic mode
	coop.opts.isAutomatic = input.IsAutomatic

	// Update the opening condition
	coop.opts.openingCondition = input.OpeningCondition

	// Update the closing condition
	coop.opts.closingCondition = input.ClosingCondition

	return nil
}

// Open opens the chicken coop.
func (coop *Coop) Open() error {
	// Check the automatic mode
	if coop.opts.isAutomatic {
		return ErrAutomaticModeEnabled
	}

	return coop.open()
}

func (coop *Coop) open() error {
	// Check the incompatible statuses
	switch coop.status {
	case Unknown:
		return fmt.Errorf("cannot open the coop because the status unknown")
	case Opened:
		return fmt.Errorf("coop is already opened")
	case Opening:
		return ErrCoopAlreadyOpening
	case Closing:
		return ErrCoopAlreadyClosing
	}

	// Update the status of the coop
	coop.status = Opening

	// Open the door
	err := coop.door.Open()
	if err != nil {
		// Update the status of the coop
		coop.status = Unknown

		return fmt.Errorf("error while opening the door: %s", err)
	}

	// Update the status of the coop
	coop.status = Opened

	return nil
}

// Close closes the chicken coop.
func (coop *Coop) Close() error {
	// Check the automatic mode
	if coop.opts.isAutomatic {
		return fmt.Errorf("cannot close the coop because automatic mode is set")
	}

	return coop.close()
}

func (coop *Coop) close() error {
	// Check the incompatible statuses
	switch coop.status {
	case Unknown:
		return fmt.Errorf("cannot open the coop because the status unknown")
	case Closed:
		return fmt.Errorf("coop is already closed")
	case Opening:
		return fmt.Errorf("coop is already opening")
	case Closing:
		return fmt.Errorf("coop is already closing")
	}

	// Update the status of the coop
	coop.status = Closing

	// Close the door
	err := coop.door.Close()
	if err != nil {
		// Update the status of the coop
		coop.status = Unknown

		return fmt.Errorf("error while opening the door: %s", err)
	}

	// Update the status of the coop
	coop.status = Closed

	return nil
}

// Check performs a check of the door of the chicken coop.
func (coop *Coop) Check() {
	// Check the automatic mode
	if !coop.opts.isAutomatic {
		logrus.WithFields(logrus.Fields{
			"status": coop.status,
		}).Warningln("Automatic mode is disabled")
		return
	}

	logrus.WithFields(logrus.Fields{
		"status":       coop.status,
		"opening_time": coop.opts.openingCondition.OpeningTime(),
		"closing_time": coop.opts.closingCondition.ClosingTime(),
	}).Debugln("Checking the coop")

	// Process the status
	switch coop.status {
	case Unknown:
		logrus.Warningln("The status is unknown")
	case Opening:
		logrus.Infoln("The coop is opening")
	case Closing:
		logrus.Infoln("The coop is closing")
	case Closed:
		if coop.shouldBeOpened(time.Now()) {
			logrus.WithFields(logrus.Fields{
				"status":       coop.status,
				"opening_time": coop.opts.openingCondition.OpeningTime(),
				"closing_time": coop.opts.closingCondition.ClosingTime(),
			}).Warnln("The coop should be opened")

			// Open the coop
			err := coop.open()
			if err != nil {
				logrus.Errorf("error while opening the coop: %s", err)
				return
			}

			logrus.Infoln("The coop has been opened")
		}
	case Opened:
		if coop.shouldBeClosed(time.Now()) {
			logrus.WithFields(logrus.Fields{
				"status":       coop.status,
				"opening_time": coop.opts.openingCondition.OpeningTime(),
				"closing_time": coop.opts.closingCondition.ClosingTime(),
			}).Warnln("The coop should be closed")

			// Close the coop
			err := coop.close()
			if err != nil {
				logrus.Errorf("Error when closing the coop: %s", err)
				return
			}

			logrus.Infoln("The coop has been closed")
		}
	default:
		logrus.Errorf("Wrong status for the coop : %s", coop.status)
	}

	logrus.WithFields(logrus.Fields{
		"status":       coop.status,
		"opening_time": coop.opts.openingCondition.OpeningTime(),
		"closing_time": coop.opts.closingCondition.ClosingTime(),
	}).Debugln("Coop has been checked")
}
