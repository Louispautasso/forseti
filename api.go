package sytralrt

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/contrib/ginrus"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type DeparturesResponse struct {
	Message    string       `json:"message,omitempty"`
	Departures *[]Departure `json:"departures,omitempty"` // the pointer allow us to display an empty array in json
}

// StatusResponse defines the object returned by the /status endpoint
type StatusResponse struct {
	Status              string    `json:"status,omitempty"`
	LastDepartureUpdate time.Time `json:"last_departure_update"`
	LastParkingUpdate   time.Time `json:"last_parking_update"`
	LastEquipMentUpdate time.Time `json:"last_equipment_update"`
}

// ParkingResponse defines how a parking object is represent in a response
type ParkingResponse struct {
	ID                        string    `json:"car_park_id"`
	UpdatedTime               time.Time `json:"updated_time"`
	AvailableSpaces           int       `json:"available"`
	OccupiedSpaces            int       `json:"occupied"`
	AvailableAccessibleSpaces int       `json:"available_PRM"`
	OccupiedAccessibleSpaces  int       `json:"occupied_PRM"`
}

// ParkingModelToResponse converts the model of a Parking object into it's view in the response
func ParkingModelToResponse(p Parking) ParkingResponse {
	return ParkingResponse{
		ID:                        p.ID,
		UpdatedTime:               p.UpdatedTime,
		AvailableSpaces:           p.AvailableStandardSpaces,
		OccupiedSpaces:            p.TotalStandardSpaces - p.AvailableStandardSpaces,
		AvailableAccessibleSpaces: p.AvailableAccessibleSpaces,
		OccupiedAccessibleSpaces:  p.TotalAccessibleSpaces - p.AvailableAccessibleSpaces,
	}
}

type ByParkingResponseId []ParkingResponse

func (p ByParkingResponseId) Len() int           { return len(p) }
func (p ByParkingResponseId) Less(i, j int) bool { return p[i].ID < p[j].ID }
func (p ByParkingResponseId) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// ParkingsResponse defines the structure returned by the /parkings endpoint
type ParkingsResponse struct {
	Parkings []ParkingResponse `json:"records,omitempty"`
	Errors   []string          `json:"errors,omitempty"`
}

// EquipmentResponse defines how a parking object is represent in a response
type EquipmentResponse struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	EmbeddedType string              `json:"embedded_type"`
	CA           CurrentAvailability `json:"current_availaibity"`
}

type CurrentAvailability struct {
	Status  string   `json:"status"`
	Cause   CauseST  `json:"cause"`
	Effect  EffectST `json:"effect"`
	Periods Period   `json:"periods"`
}

type CauseST struct {
	Label string `json:"label"`
}

type EffectST struct {
	Label string `json:"label"`
}

type Period struct {
	Begin time.Time `json:"begin"`
	End   time.Time `json:"end"`
}

// EquipmentModelToResponse converts the model of a Equipment object into it's view in the response
func EquipmentModelToResponse(e Equipment) EquipmentResponse {
	return EquipmentResponse{
		ID:           e.ID,
		Name:         e.Name,
		EmbeddedType: e.EmbeddedType,
		CA: CurrentAvailability{Status: GetAvailabilityStatus(e.Start, e.End), Cause: CauseST{Label: e.Cause},
			Effect: EffectST{Label: e.Effect}, Periods: Period{Begin: e.Start, End: e.End}},
	}
}

// EquipmentsResponse defines the structure returned by the /equipments endpoint
type EquipmentsResponse struct {
	Equipments []EquipmentResponse `json:"equipments_details,omitempty"`
	Errors     []string            `json:"errors,omitempty"`
}

var (
	httpDurations = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "sytralrt",
		Subsystem: "http",
		Name:      "durations_seconds",
		Help:      "http request latency distributions.",
		Buckets:   prometheus.ExponentialBuckets(0.001, 1.5, 15),
	},
		[]string{"handler", "code"},
	)

	httpInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "sytralrt",
		Subsystem: "http",
		Name:      "in_flight",
		Help:      "current number of http request being served",
	},
	)
)

func DeparturesHandler(manager *DataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		response := DeparturesResponse{}
		stopID := c.Query("stop_id")
		if stopID == "" {
			response.Message = "stopID is required"
			c.JSON(http.StatusBadRequest, response)
			return
		}
		departures, err := manager.GetDeparturesByStop(stopID)
		if err != nil {
			response.Message = "No data loaded"
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}
		response.Departures = &departures
		c.JSON(http.StatusOK, response)
	}
}

func StatusHandler(manager *DataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, StatusResponse{
			"ok",
			manager.GetLastDepartureDataUpdate(),
			manager.GetLastParkingsDataUpdate(),
			manager.GetLastEquipmentsDataUpdate(),
		})
	}
}

func ParkingsHandler(manager *DataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			parkings []Parking
			errStr   []string
		)

		if ids, ok := c.GetQueryArray("ids[]"); ok {
			// Only query parkings with a specific id
			var errs []error
			parkings, errs = manager.GetParkingsByIds(ids)
			for _, e := range errs {
				errStr = append(errStr, e.Error())
			}
		} else {
			// Query all parkings !
			var err error
			parkings, err = manager.GetParkings()
			if err != nil {
				errStr = append(errStr, err.Error())
			}
		}

		// Convert Parkings from the model to a response view
		parkingsResp := make([]ParkingResponse, len(parkings))
		for i, p := range parkings {
			parkingsResp[i] = ParkingModelToResponse(p)
		}
		c.JSON(http.StatusOK, ParkingsResponse{
			Parkings: parkingsResp,
			Errors:   errStr,
		})
	}
}

func EquipmentsHandler(manager *DataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			equipments []Equipment
			errStr     []string
		)

		// Query all equipments !
		var err error
		equipments, err = manager.GetEquipments()
		if err != nil {
			errStr = append(errStr, err.Error())
		}

		// Convert Equipments from the model to a response view
		equipmentsResp := make([]EquipmentResponse, len(equipments))
		for i, e := range equipments {
			equipmentsResp[i] = EquipmentModelToResponse(e)
		}
		c.JSON(http.StatusOK, EquipmentsResponse{
			Equipments: equipmentsResp,
			Errors:     errStr,
		})
	}
}

func SetupRouter(manager *DataManager, r *gin.Engine) *gin.Engine {
	if r == nil {
		r = gin.New()
	}
	r.Use(ginrus.Ginrus(logrus.StandardLogger(), time.RFC3339, false))
	r.Use(instrumentGin())
	r.Use(gin.Recovery())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/departures", DeparturesHandler(manager))
	r.GET("/status", StatusHandler(manager))
	r.GET("/parkings/P+R", ParkingsHandler(manager))
	r.GET("/equipments", EquipmentsHandler(manager))

	return r
}

func instrumentGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		begin := time.Now()
		httpInFlight.Inc()
		c.Next()
		httpInFlight.Dec()
		observer := httpDurations.With(prometheus.Labels{"handler": c.HandlerName(), "code": strconv.Itoa(c.Writer.Status())})
		observer.Observe(time.Since(begin).Seconds())
	}
}

func init() {
	prometheus.MustRegister(httpDurations)
	prometheus.MustRegister(httpInFlight)
}
