package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type PostRequestBody struct {
	Product_id      string  `json:"product_id"`
	Start_latitude  float64 `json:"start_latitude"`
	Start_longitude float64 `json:"start_longitude"`
	End_latitude    float64 `json:"end_latitude"`
	End_longitude   float64 `json:"end_longitude"`
}

type PutRequestBody struct {
	Status string `json:"status"`
}

type TripRequest struct {
	Starting_from_location_id string
	Location_ids              []string
}

type TripResponse struct {
	Id                        int64
	Status                    string
	Starting_from_location_id string
	Best_route_location_ids   []string
	Total_uber_costs          float64
	Total_uber_duration       float64
	Total_distance            float64
}

type TripProductIds struct {
	Id          int64
	Product_Ids []string
}

type PutTripResponse struct {
	Id                           int64
	Status                       string
	Starting_from_location_id    string
	Next_destination_location_id string
	Best_route_location_ids      []string
	Total_uber_costs             float64
	Total_uber_duration          float64
	Total_distance               float64
	Uber_wait_time_eta           float64
}

type LocationRequest struct {
	Name    string
	Address string
	City    string
	State   string
	Zip     string
}

type LocationLatLng struct {
	Lat float64
	Lng float64
}

type LocationResponse struct {
	Id         int64
	Name       string
	Address    string
	City       string
	State      string
	Zip        string
	Coordinate LocationLatLng
}

const (
	MongoDBHosts = "ds043694.mongolab.com:43694"
	AuthDatabase = "locationdb"
	AuthUserName = "mockrunuser"
	AuthPassword = "mockrunuser@273"
	TestDatabase = "locationdb"
	AccessToken  = ""
)

func main() {

	mux := httprouter.New()
	mux.POST("/locations", createLocation)
	mux.GET("/locations/:location_id", getLocation)
	mux.PUT("/locations/:location_id", updateLocation)
	mux.DELETE("/locations/:location_id", removeLocation)

	mux.POST("/trips", planAtrip)
	mux.GET("/trips/:trip_id", checkTripDetails)
	mux.PUT("/trips/:trip_id/request", requestTrip)

	server := http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}
	server.ListenAndServe()
}

// Create Trip Id - POST Request
func planAtrip(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {

	//Connect to mongoDB
	info := &mgo.DialInfo{
		Addrs:    []string{MongoDBHosts},
		Timeout:  60 * time.Second,
		Database: AuthDatabase,
		Username: AuthUserName,
		Password: AuthPassword,
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	// Optional. Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	db := session.DB("locationdb")
	collection := db.C("LocationCollection")

	// Read input parameters
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic("Error in reading body.")
	}

	var tripResp TripResponse
	var tripReq TripRequest
	err = json.Unmarshal(body, &tripReq)
	if err != nil {
		panic(err)
	}

	var latitude []float64
	var longitude []float64
	var locationId []int64

	// Starting location Id
	tripResp.Starting_from_location_id = tripReq.Starting_from_location_id
	location_id, _ := strconv.ParseInt(tripReq.Starting_from_location_id, 10, 64)

	// Search for the location_id and find its corresponding coordinates
	var locResp LocationResponse
	err = collection.Find(bson.M{"id": location_id}).One(&locResp)
	if err != nil {
		log.Fatal(err)
	}

	startLocLat := locResp.Coordinate.Lat
	startLocLng := locResp.Coordinate.Lng

	latitude = append(latitude, locResp.Coordinate.Lat)
	longitude = append(longitude, locResp.Coordinate.Lng)
	locationId = append(locationId, locResp.Id)

	// variable for iterating the maps in the order of insertion of values
	var globalOrder int64
	globalOrder = 0

	bestRouteCost := make(map[int64]float64)
	bestRouteDistance := make(map[int64]float64)
	bestRouteDuration := make(map[int64]float64)
	bestRouteProduct := make(map[int64]string)
	bestRouteLat := make(map[int64]float64)
	bestRouteLng := make(map[int64]float64)

	bestRouteCost[locResp.Id] = -1
	bestRouteLat[locResp.Id] = startLocLat
	bestRouteLng[locResp.Id] = startLocLng

	orderedBestRoute := make(map[int64]int64)
	orderedBestRoute[globalOrder] = locResp.Id

	// Finding coordinates of all the locations
	for i := 0; i < len(tripReq.Location_ids); i++ {
		location_id, _ := strconv.ParseInt(tripReq.Location_ids[i], 10, 64)

		var locResp LocationResponse
		err = collection.Find(bson.M{"id": location_id}).One(&locResp)
		if err != nil {
			log.Fatal(err)
		}

		latitude = append(latitude, locResp.Coordinate.Lat)
		longitude = append(longitude, locResp.Coordinate.Lng)
		locationId = append(locationId, locResp.Id)
	}

	startLat := latitude[0]
	startLng := longitude[0]

	// Find the best route based on cost
	for z := 0; z < len(tripReq.Location_ids); z++ {

		minCost := make(map[int64]float64)
		minDistance := make(map[int64]float64)
		minDuration := make(map[int64]float64)
		minLat := make(map[int64]float64)
		minLng := make(map[int64]float64)
		prodId := make(map[int64]string)

		for j := 0; j < len(latitude); j++ {
			// Condition to skip starting location id and location ids which are already in best route
			if startLat != latitude[j] && startLng != longitude[j] && bestRouteCost[locationId[j]] == 0.0 {

				startLatStr := strconv.FormatFloat(startLat, 'f', 7, 64)
				startLngStr := strconv.FormatFloat(startLng, 'f', 7, 64)
				endLatStr := strconv.FormatFloat(latitude[j], 'f', 7, 64)
				endLngStr := strconv.FormatFloat(longitude[j], 'f', 7, 64)

				client := &http.Client{}
				request, _ := http.NewRequest("GET", "https://sandbox-api.uber.com/v1/estimates/price?start_latitude="+startLatStr+"&start_longitude="+startLngStr+"&end_latitude="+endLatStr+"&end_longitude="+endLngStr, nil)
				request.Header.Set("Authorization", "Token JaiBIl-1gB5pqBkMa0dgPANW4e5MqJ_1AyoTQ0AV")
				resp, err := client.Do(request)
				if err != nil {
					fmt.Println("Error")
				}
				defer resp.Body.Close()

				body, _ = ioutil.ReadAll(resp.Body)
				var msgRes interface{}
				_ = json.Unmarshal(body, &msgRes)

				mRes := msgRes.(map[string]interface{})["prices"].([]interface{})
				var minCst, minDist, minDur float64
				var minCstProdId string

				// Iterating to find the lowest estimate uber to all location ids
				//from the current starting location id
				for k := 0; k < len(mRes); k++ {

					if mRes[k].(map[string]interface{})["low_estimate"] != nil {

						lowEstimate := mRes[k].(map[string]interface{})["low_estimate"].(float64)
						distance := mRes[k].(map[string]interface{})["distance"].(float64)
						duration := mRes[k].(map[string]interface{})["duration"].(float64)
						productId := mRes[k].(map[string]interface{})["product_id"].(string)
						cost := lowEstimate
						if k == 0 {
							minCst = cost
							minDist = distance
							minDur = duration
							minCstProdId = productId

						} else {
							if minCst > cost {
								minCst = cost
								minDist = distance
								minDur = duration
								minCstProdId = productId
							}
						}

					}

				}
				minCost[locationId[j]] = minCst
				minDuration[locationId[j]] = minDur
				minDistance[locationId[j]] = minDist
				prodId[locationId[j]] = minCstProdId
				minLat[locationId[j]] = latitude[j]
				minLng[locationId[j]] = longitude[j]
			}
		}

		// Find the best possible next destination depending on the cost
		var bestCst, bestDst, bestDr, bestLat, bestLng float64
		var bestProdId string
		var locId int64
		bestCst = 1.797693134862315708145274237317043567981e+308
		bestDr = 1.797693134862315708145274237317043567981e+308
		for k, v := range minCost {
			if bestCst > v {
				bestCst = v
				locId = k
				bestDst = minDistance[k]
				bestDr = minDuration[k]
				bestProdId = prodId[k]
				bestLat = minLat[k]
				bestLng = minLng[k]
			} else if bestCst == v {
				if bestDr > minDuration[k] {
					bestCst = v
					locId = k
					bestDst = minDistance[k]
					bestDr = minDuration[k]
					bestProdId = prodId[k]
					bestLat = minLat[k]
					bestLng = minLng[k]
				}
			}
		}

		if locId != 0 {
			globalOrder = globalOrder + 1
			orderedBestRoute[globalOrder] = locId
			bestRouteCost[locId] = bestCst
			bestRouteDistance[locId] = bestDst
			bestRouteDuration[locId] = bestDr
			bestRouteProduct[locId] = bestProdId
			bestRouteLat[locId] = bestLat
			bestRouteLng[locId] = bestLng

			startLat = bestLat
			startLng = bestLng
		}
	}

	// To get the ordered best route, total cost, total duration and distance
	bestRouteArr := []string{}
	bestProductArr := []string{}
	var totalUberCost, totalUberDistance, totalUberDuration float64
	var i, lenI int64

	totalUberCost = 0
	totalUberDistance = 0
	totalUberDuration = 0
	lenI = int64(len(orderedBestRoute))

	for i = 1; i < lenI; i++ {
		totalUberCost += bestRouteCost[orderedBestRoute[i]]
		totalUberDistance += bestRouteDistance[orderedBestRoute[i]]
		totalUberDuration += bestRouteDuration[orderedBestRoute[i]]
		bestRouteArr = append(bestRouteArr, strconv.Itoa(int(orderedBestRoute[i])))
		bestProductArr = append(bestProductArr, bestRouteProduct[orderedBestRoute[i]])
	}

	// Code to calculate distance, cost, duration back to starting location
	startLatStr := strconv.FormatFloat(bestRouteLat[orderedBestRoute[lenI-1]], 'f', 7, 64)
	startLngStr := strconv.FormatFloat(bestRouteLng[orderedBestRoute[lenI-1]], 'f', 7, 64)
	endLatStr := strconv.FormatFloat(startLocLat, 'f', 7, 64)
	endLngStr := strconv.FormatFloat(startLocLng, 'f', 7, 64)

	client := &http.Client{}
	request, _ := http.NewRequest("GET", "https://sandbox-api.uber.com/v1/estimates/price?start_latitude="+startLatStr+"&start_longitude="+startLngStr+"&end_latitude="+endLatStr+"&end_longitude="+endLngStr, nil)
	request.Header.Set("Authorization", "Token JaiBIl-1gB5pqBkMa0dgPANW4e5MqJ_1AyoTQ0AV")
	resp, err := client.Do(request)
	if err != nil {
		fmt.Println("Error")
	}
	defer resp.Body.Close()

	body, _ = ioutil.ReadAll(resp.Body)
	var msgRes interface{}
	_ = json.Unmarshal(body, &msgRes)

	mRes := msgRes.(map[string]interface{})["prices"].([]interface{})
	var minCst, minDist, minDur float64

	// Iterating to find the lowest estimate uber to all location ids
	//from the current starting location id
	for k := 0; k < len(mRes); k++ {

		if mRes[k].(map[string]interface{})["low_estimate"] != nil {

			lowEstimate := mRes[k].(map[string]interface{})["low_estimate"].(float64)
			distance := mRes[k].(map[string]interface{})["distance"].(float64)
			duration := mRes[k].(map[string]interface{})["duration"].(float64)
			cost := lowEstimate
			if k == 0 {
				minCst = cost
				minDist = distance
				minDur = duration

			} else {
				if minCst > cost {
					minCst = cost
					minDist = distance
					minDur = duration
				}
			}

		}

	}

	tripResp.Total_distance = totalUberDistance + minDist
	tripResp.Total_uber_costs = totalUberCost + minCst
	tripResp.Total_uber_duration = totalUberDuration + minDur
	tripResp.Best_route_location_ids = bestRouteArr
	tripResp.Status = "planning"

	var searchId TripResponse
	collection = db.C("TripsCollection")
	err = collection.Find(nil).Sort("-id").One(&searchId)
	if err != nil {
		tripResp.Id = 1122
	} else {
		tripResp.Id = searchId.Id + 1
	}

	err = collection.Insert(tripResp)
	if err != nil {
		log.Fatal(err)
	} else {
		mapB, _ := json.Marshal(tripResp)
		fmt.Fprintf(rw, "\n"+string(mapB)+"\n")
	}
}

// Request Trip - PUT Method
func requestTrip(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {
	// Connect to mongoDB
	info := &mgo.DialInfo{
		Addrs:    []string{MongoDBHosts},
		Timeout:  60 * time.Second,
		Database: AuthDatabase,
		Username: AuthUserName,
		Password: AuthPassword,
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	// Optional. Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	db := session.DB("locationdb")
	collection := db.C("TripsCollection")

	// Fetch Input tripId
	trip_idStr := p.ByName("trip_id")
	trip_id, _ := strconv.ParseInt(trip_idStr, 10, 64)

	var tripResp TripResponse

	// Search for the trip_id
	err = collection.Find(bson.M{"id": trip_id}).One(&tripResp)
	if err != nil {
		fmt.Fprintf(rw, "\nNo such trip_id found!\n")
		return
	}

	collection = db.C("PutTripCollection")
	var putTripResp PutTripResponse

	// Search for the trip_id
	err = collection.Find(bson.M{"id": trip_id}).One(&putTripResp)
	if err != nil {
		putTripResp.Id = tripResp.Id
		putTripResp.Best_route_location_ids = tripResp.Best_route_location_ids
		putTripResp.Starting_from_location_id = tripResp.Starting_from_location_id
		putTripResp.Total_distance = tripResp.Total_distance
		putTripResp.Total_uber_costs = tripResp.Total_uber_costs
		putTripResp.Total_uber_duration = tripResp.Total_uber_duration
	}

	var startLocId, endLocId string

	if putTripResp.Next_destination_location_id == "" {
		putTripResp.Status = "requesting"
		startLocId = putTripResp.Starting_from_location_id
		putTripResp.Next_destination_location_id = putTripResp.Best_route_location_ids[0]
	} else {
		for i := 0; i < len(putTripResp.Best_route_location_ids); i++ {
			if putTripResp.Best_route_location_ids[i] == putTripResp.Next_destination_location_id {
				if i == len(putTripResp.Best_route_location_ids)-1 {
					putTripResp.Status = "reached"
					startLocId = putTripResp.Next_destination_location_id
					putTripResp.Next_destination_location_id = putTripResp.Starting_from_location_id
				} else {
					putTripResp.Status = "requesting"
					startLocId = putTripResp.Next_destination_location_id
					putTripResp.Next_destination_location_id = putTripResp.Best_route_location_ids[i+1]
				}
				break
			}
		}
	} //else ends for empty next destination id

	endLocId = putTripResp.Next_destination_location_id

	startlocation_id, _ := strconv.ParseInt(startLocId, 10, 64)
	endLocation_id, _ := strconv.ParseInt(endLocId, 10, 64)

	// Search for the location_id and find its corresponding coordinates
	var locResp LocationResponse
	collection = db.C("LocationCollection")
	err = collection.Find(bson.M{"id": startlocation_id}).One(&locResp)
	if err != nil {
		fmt.Fprintf(rw, "\nTrip already completed.\n")
		return
	}
	startLat := locResp.Coordinate.Lat
	startLng := locResp.Coordinate.Lng
	startLatStr := strconv.FormatFloat(locResp.Coordinate.Lat, 'f', 7, 64)
	startLngStr := strconv.FormatFloat(locResp.Coordinate.Lng, 'f', 7, 64)

	err = collection.Find(bson.M{"id": endLocation_id}).One(&locResp)
	if err != nil {
		log.Fatal(err)
	}

	endLat := locResp.Coordinate.Lat
	endLng := locResp.Coordinate.Lng
	endLatStr := strconv.FormatFloat(locResp.Coordinate.Lat, 'f', 7, 64)
	endLngStr := strconv.FormatFloat(locResp.Coordinate.Lng, 'f', 7, 64)

	client := &http.Client{}
	request, _ := http.NewRequest("GET", "https://sandbox-api.uber.com/v1/estimates/price?start_latitude="+startLatStr+"&start_longitude="+startLngStr+"&end_latitude="+endLatStr+"&end_longitude="+endLngStr, nil)
	request.Header.Set("Authorization", "Token JaiBIl-1gB5pqBkMa0dgPANW4e5MqJ_1AyoTQ0AV")
	resp, err := client.Do(request)
	if err != nil {
		fmt.Println("Error")
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var msgRes interface{}
	_ = json.Unmarshal(body, &msgRes)

	mRes := msgRes.(map[string]interface{})["prices"].([]interface{})
	var minCst float64
	var minCstProdId string

	// Iterating to find the lowest estimate uber to all location ids
	//from the current starting location id
	for k := 0; k < len(mRes); k++ {
		if mRes[k].(map[string]interface{})["low_estimate"] != nil {
			lowEstimate := mRes[k].(map[string]interface{})["low_estimate"].(float64)
			productId := mRes[k].(map[string]interface{})["product_id"].(string)
			cost := lowEstimate
			if k == 0 {
				minCst = cost
				minCstProdId = productId

			} else {
				if minCst > cost {
					minCst = cost
					minCstProdId = productId
				}
			}
		}
	}

	// POST REQUEST
	res1D := &PostRequestBody{
		Product_id:      minCstProdId,
		Start_latitude:  startLat,
		Start_longitude: startLng,
		End_latitude:    endLat,
		End_longitude:   endLng}

	res1B, _ := json.Marshal(res1D)

	request, _ = http.NewRequest("POST", "https://sandbox-api.uber.com/v1/requests", bytes.NewBuffer(res1B))
	request.Header.Set("Authorization", "Bearer "+AccessToken)
	request.Header.Set("Content-Type", "application/json")
	Client1 := &http.Client{}
	resp, err = Client1.Do(request)
	if err != nil {
		fmt.Println("Error")
		return
	}
	defer resp.Body.Close()

	body, _ = ioutil.ReadAll(resp.Body)
	var msgResPost interface{}
	_ = json.Unmarshal(body, &msgResPost)

	msgRequestId := msgResPost.(map[string]interface{})["request_id"].(string)
	msgETA := msgResPost.(map[string]interface{})["eta"].(float64)

	res2D := &PutRequestBody{
		Status: "completed"}
	res2B, _ := json.Marshal(res2D)

	request, _ = http.NewRequest("PUT", "https://sandbox-api.uber.com/v1/sandbox/requests/"+msgRequestId, bytes.NewBuffer(res2B))
	request.Header.Set("Authorization", "Bearer "+AccessToken)
	request.Header.Set("Content-Type", "application/json")
	Client1 = &http.Client{}
	resp, err = Client1.Do(request)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		putTripResp.Uber_wait_time_eta = msgETA

		var searchPutTripResp PutTripResponse
		// Search for the trip_id
		collection = db.C("PutTripCollection")
		err = collection.Find(bson.M{"id": trip_id}).One(&searchPutTripResp)
		if err != nil {
			err = collection.Insert(putTripResp)
			if err != nil {
				log.Fatal(err)
			} else {
				mapB, _ := json.Marshal(putTripResp)
				fmt.Fprintf(rw, "\n"+string(mapB)+"\n")
			}
		} else {
			err = collection.Update(
				bson.M{"id": trip_id},
				bson.M{"$set": bson.M{
					"status":                       putTripResp.Status,
					"next_destination_location_id": putTripResp.Next_destination_location_id,
					"uber_wait_time_eta":           putTripResp.Uber_wait_time_eta}})
			if err != nil {
				log.Fatal(err)
			} else {
				mapB, _ := json.Marshal(putTripResp)
				fmt.Fprintf(rw, "\n"+string(mapB)+"\n")
			}
		}

	} else {
		fmt.Println("Different Response Status Code for request api call.")
	}

}

// Get Trip Details - GET Method
func checkTripDetails(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {

	// Connect to mongoDB
	info := &mgo.DialInfo{
		Addrs:    []string{MongoDBHosts},
		Timeout:  60 * time.Second,
		Database: AuthDatabase,
		Username: AuthUserName,
		Password: AuthPassword,
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	// Optional. Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	db := session.DB("locationdb")
	collection := db.C("TripsCollection")

	// Fetch Input location_id
	trip_idStr := p.ByName("trip_id")
	trip_id, _ := strconv.ParseInt(trip_idStr, 10, 64)

	var tripResp TripResponse

	// Search for the location_id
	err = collection.Find(bson.M{"id": trip_id}).One(&tripResp)
	if err != nil {
		log.Fatal(err)
	}

	mapB, _ := json.Marshal(tripResp)
	fmt.Fprintf(rw, "\n"+string(mapB)+"\n")
}

// Create Location - POST Method
func createLocation(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {

	// Connect to mongoDB server
	info := &mgo.DialInfo{
		Addrs:    []string{MongoDBHosts},
		Timeout:  60 * time.Second,
		Database: AuthDatabase,
		Username: AuthUserName,
		Password: AuthPassword,
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	// Optional. Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	db := session.DB("locationdb")
	collection := db.C("LocationCollection")

	// Fetch Input Json
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic("Error in reading body.")
	}

	var loc LocationRequest
	err = json.Unmarshal(body, &loc)
	if err != nil {
		panic("Error in unmarshalling.")
	}

	var locResp LocationResponse
	locResp.Name = loc.Name
	locResp.Address = loc.Address
	locResp.City = loc.City
	locResp.State = loc.State
	locResp.Zip = loc.Zip

	var address string
	address = ""

	if loc.Name != "" {
		address = address + loc.Name
	}
	if loc.Address != "" {
		address = address + ", " + loc.Address
	}
	if loc.City != "" {
		address = address + ", " + loc.City
	}
	if loc.State != "" {
		address = address + ", " + loc.State
	}
	if loc.Zip != "" {
		address = address + ", " + loc.Zip
	}

	address = strings.Replace(address, " ", "+", -1)

	// Fetch Coordinates
	resp, err := http.Get("http://maps.google.com/maps/api/geocode/json?address=" + address + "&sensor=false")
	if err != nil {
		fmt.Println("Error")
	}
	defer resp.Body.Close()

	body, _ = ioutil.ReadAll(resp.Body)
	var msgRes interface{}
	_ = json.Unmarshal(body, &msgRes)

	mRes := msgRes.(map[string]interface{})["results"]
	mRes0 := mRes.([]interface{})[0]
	mGeo := mRes0.(map[string]interface{})["geometry"]
	mLoc := mGeo.(map[string]interface{})["location"]

	locLat := mLoc.(map[string]interface{})["lat"].(float64)
	locLng := mLoc.(map[string]interface{})["lng"].(float64)

	var locCoordinates LocationLatLng
	locCoordinates.Lat = locLat
	locCoordinates.Lng = locLng

	locResp.Coordinate = locCoordinates

	// Check if documents exists
	var searchId LocationResponse
	err = collection.Find(nil).Sort("-id").One(&searchId)
	if err != nil {
		locResp.Id = 12345
	} else {
		locResp.Id = searchId.Id + 1
	}

	err = collection.Insert(locResp)
	if err != nil {
		log.Fatal(err)
	} else {
		mapB, _ := json.Marshal(locResp)
		fmt.Fprintf(rw, "\n"+string(mapB)+"\n")
	}
}

// Get Location - GET Method
func getLocation(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {

	// Connect to mongoDB
	info := &mgo.DialInfo{
		Addrs:    []string{MongoDBHosts},
		Timeout:  60 * time.Second,
		Database: AuthDatabase,
		Username: AuthUserName,
		Password: AuthPassword,
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	// Optional. Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	db := session.DB("locationdb")
	collection := db.C("LocationCollection")

	// Fetch Input location_id
	location_idStr := p.ByName("location_id")
	location_id, _ := strconv.ParseInt(location_idStr, 10, 64)

	var locResp LocationResponse

	// Search for the location_id
	err = collection.Find(bson.M{"id": location_id}).One(&locResp)
	if err != nil {
		log.Fatal(err)
	}

	mapB, _ := json.Marshal(locResp)
	fmt.Fprintf(rw, "\n"+string(mapB)+"\n")
}

// Update Location Address - PUT Method
func updateLocation(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {

	// Connect to mongoDB
	info := &mgo.DialInfo{
		Addrs:    []string{MongoDBHosts},
		Timeout:  60 * time.Second,
		Database: AuthDatabase,
		Username: AuthUserName,
		Password: AuthPassword,
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	// Optional. Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	db := session.DB("locationdb")
	collection := db.C("LocationCollection")

	// Fetch Input Json
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic("Error in reading body.")
	}

	var loc LocationRequest
	err = json.Unmarshal(body, &loc)
	if err != nil {
		panic("Error in unmarshalling.")
	}

	// Fetch Input location_id
	location_idStr := p.ByName("location_id")
	location_id, _ := strconv.ParseInt(location_idStr, 10, 64)

	address := loc.Address + ", " + loc.City + ", " + loc.State
	address = strings.Replace(address, " ", "+", -1)

	// Fetch Coordinates
	resp, err := http.Get("http://maps.google.com/maps/api/geocode/json?address=" + address + "&sensor=false")
	if err != nil {
		fmt.Println("Error")
	}
	defer resp.Body.Close()

	body, _ = ioutil.ReadAll(resp.Body)
	var msgRes interface{}
	_ = json.Unmarshal(body, &msgRes)

	mRes := msgRes.(map[string]interface{})["results"]
	mRes0 := mRes.([]interface{})[0]
	mGeo := mRes0.(map[string]interface{})["geometry"]
	mLoc := mGeo.(map[string]interface{})["location"]

	locLat := mLoc.(map[string]interface{})["lat"].(float64)
	locLng := mLoc.(map[string]interface{})["lng"].(float64)

	// Search for the location_id & update the location details
	err = collection.Update(bson.M{"id": location_id}, bson.M{"$set": bson.M{"address": loc.Address, "city": loc.City, "state": loc.State, "zip": loc.Zip, "coordinate.lat": locLat, "coordinate.lng": locLng}})
	if err != nil {
		log.Fatal(err)
	}

	var locResp LocationResponse

	// Search for the updated location_id
	err = collection.Find(bson.M{"id": location_id}).One(&locResp)
	if err != nil {
		log.Fatal(err)
	}

	mapB, _ := json.Marshal(locResp)
	fmt.Fprintf(rw, "\n"+string(mapB)+"\n")
}

// Remove Location - DELETE Method
func removeLocation(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {

	// Connect to mongoDB
	info := &mgo.DialInfo{
		Addrs:    []string{MongoDBHosts},
		Timeout:  60 * time.Second,
		Database: AuthDatabase,
		Username: AuthUserName,
		Password: AuthPassword,
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	// Optional. Switch the session to a monotonic behavior.
	session.SetMode(mgo.Monotonic, true)

	db := session.DB("locationdb")
	collection := db.C("LocationCollection")

	// Fetch Input location_id
	location_idStr := p.ByName("location_id")
	location_id, _ := strconv.ParseInt(location_idStr, 10, 64)

	// Delete the location corresponding to the location_id
	err = collection.Remove(bson.M{"id": location_id})
	if err != nil {
		log.Fatal(err)
	} else {
		fmt.Fprintf(rw, "Location document deleted successfully.\n")
	}
}
