package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/CanalTP/forseti"
	"github.com/CanalTP/forseti/internal/departures"
	"github.com/CanalTP/forseti/internal/equipments"
	"github.com/CanalTP/forseti/internal/freefloatings"
	"github.com/CanalTP/forseti/internal/manager"
	"github.com/CanalTP/forseti/internal/parkings"
	"github.com/CanalTP/forseti/internal/vehicleoccupancies"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/ginrus"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type LoadingStatus struct {
	RefreshActive bool      `json:"refresh_active"`
	LastUpdate    time.Time `json:"last_update"`
}

// StatusResponse defines the object returned by the /status endpoint
type StatusResponse struct {
	Status              string        `json:"status,omitempty"`
	Version             string        `json:"version,omitempty"`
	LastDepartureUpdate time.Time     `json:"last_departure_update"`
	LastParkingUpdate   time.Time     `json:"last_parking_update"`
	LastEquipmentUpdate time.Time     `json:"last_equipment_update"`
	FreeFloatings       LoadingStatus `json:"free_floatings,omitempty"`
	VehicleOccupancies  LoadingStatus `json:"vehicle_occupancies,omitempty"`
}

var (
	httpDurations = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "forseti",
		Subsystem: "http",
		Name:      "durations_seconds",
		Help:      "http request latency distributions.",
		Buckets:   prometheus.ExponentialBuckets(0.001, 1.5, 15),
	},
		[]string{"handler", "code"},
	)

	httpInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "forseti",
		Subsystem: "http",
		Name:      "in_flight",
		Help:      "current number of http request being served",
	},
	)
)

func StatusHandler(manager *manager.DataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var lastFreeFloatingsDataUpdate time.Time
		var loadFreeFloatingData bool = false
		if manager.GetFreeFloatingsContext() != nil {
			// manage freefloating activation /status?free_floatings=true or false
			freeFloatingStatus := c.Query("free_floatings")
			if len(freeFloatingStatus) > 0 {
				toActive, _ := strconv.ParseBool(freeFloatingStatus)
				manager.GetFreeFloatingsContext().ManageFreeFloatingsStatus(toActive)
			}
			lastFreeFloatingsDataUpdate = manager.GetFreeFloatingsContext().GetLastFreeFloatingsDataUpdate()
			loadFreeFloatingData = manager.GetFreeFloatingsContext().LoadFreeFloatingsData()
		}

		var lastVehicleOccupanciesDataUpdate time.Time
		var loadVehicleOccupanciesData bool = false
		if manager.GetVehiculeOccupanciesContext() != nil {
			// manage vehicleoccupancy activation /status?vehicle_occupancies=true or false
			vehicleOccupancyStatus := c.Query("vehicle_occupancies")
			if len(vehicleOccupancyStatus) > 0 {
				toActive, _ := strconv.ParseBool(vehicleOccupancyStatus)
				manager.GetVehiculeOccupanciesContext().ManageVehicleOccupancyStatus(toActive)
			}
			lastVehicleOccupanciesDataUpdate = manager.GetVehiculeOccupanciesContext().GetLastVehicleOccupanciesDataUpdate()
			loadVehicleOccupanciesData = manager.GetVehiculeOccupanciesContext().LoadOccupancyData()
		}

		var lastEquipmentDataUpdate time.Time
		if manager.GetEquipmentsContext() != nil {
			lastEquipmentDataUpdate = manager.GetEquipmentsContext().GetLastEquipmentsDataUpdate()
		}

		var lastDeparturesDataUpdate time.Time
		if manager.GetDeparturesContext() != nil {
			lastDeparturesDataUpdate = manager.GetDeparturesContext().GetLastDepartureDataUpdate()
		}

		var lastParkingsDataUpdate time.Time
		if manager.GetParkingsContext() != nil {
			lastParkingsDataUpdate = manager.GetParkingsContext().GetLastParkingsDataUpdate()
		}

		c.JSON(http.StatusOK, StatusResponse{
			"ok",
			forseti.ForsetiVersion,
			lastDeparturesDataUpdate,
			lastParkingsDataUpdate,
			lastEquipmentDataUpdate,
			LoadingStatus{loadFreeFloatingData, lastFreeFloatingsDataUpdate},
			LoadingStatus{loadVehicleOccupanciesData, lastVehicleOccupanciesDataUpdate},
		})
	}
}

func SetupRouter(manager *manager.DataManager, r *gin.Engine) *gin.Engine {
	if r == nil {
		r = gin.New()
	}
	r.Use(ginrus.Ginrus(logrus.StandardLogger(), time.RFC3339, false))
	r.Use(instrumentGin())
	r.Use(gin.Recovery())
	pprof.Register(r)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/status", StatusHandler(manager))

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
	prometheus.MustRegister(departures.DepartureLoadingDuration)
	prometheus.MustRegister(departures.DepartureLoadingErrors)
	prometheus.MustRegister(parkings.ParkingsLoadingDuration)
	prometheus.MustRegister(parkings.ParkingsLoadingErrors)
	prometheus.MustRegister(equipments.EquipmentsLoadingDuration)
	prometheus.MustRegister(equipments.EquipmentsLoadingErrors)
	prometheus.MustRegister(freefloatings.FreeFloatingsLoadingDuration)
	prometheus.MustRegister(freefloatings.FreeFloatingsLoadingErrors)
	prometheus.MustRegister(vehicleoccupancies.VehicleOccupanciesLoadingDuration)
	prometheus.MustRegister(vehicleoccupancies.VehicleOccupanciesLoadingErrors)
}
