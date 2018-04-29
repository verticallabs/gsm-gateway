package main

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadTimeAsUTC(t *testing.T) {
	Convey("Converting time", t, func() {
		location, _ := time.LoadLocation("America/New_York")
		vanTime, _ := time.ParseInLocation("2006/01/02,15:04:05", "1943/09/24,10:00:00", location)

		Convey("should have the correct time in the other time zone", func() {
			timeAsUTC := readTimeAsUTC(vanTime, location).Format(time.RFC3339)
			So(timeAsUTC, ShouldEqual, "1943-09-24T14:00:00Z")
		})
	})
}
