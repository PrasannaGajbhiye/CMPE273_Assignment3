# CMPE273_Assignment3
# Trip Planner

The trip planner service will take a set of locations from the database and will then check against UBER’s price estimates API to suggest the best possible route in terms of costs and duration.

## Usage

### Install

```
go get github.com/PrasannaGajbhiye/CMPE273_Assignment3
```

After installation, create a new access token and use that value for the constant "AccessToken" in the "main.go" file. After this, proceed with the following steps.

### Start the  server:

```
go clean
go build
./manGo
```

### Start the client 

Following is the list of sample locations present in the database:

1. 12346 - Facebook, 1 Hacker Way, Menlo Park, CA 94025
2. 12347 - Googleplex, 1600 Amphitheatre Pkwy, Mountain View, CA 94043
3. 12348 - Dr. Martin Luther King, Jr. Library, 150 E San Fernando St, San Jose, CA 95112
4. 12349 - Yahoo!, 701 1st Ave Sunnyvale, CA 94089
5. 12350 - Intuit, Inc. - Building #11, 2675 Coast Ave, Mountain View, CA 94043
6. 12351 - Fairmont Hotel San Francisco,950 Mason St, San Francisco, CA 94108
7. 12352 - Golden Gate Bridge, San Francisco, CA
8. 12353 - Pier 39,Beach Street & The Embarcadero, San Francisco, CA 94133
9. 12354 - Golden Gate Park, San Francisco, CA 94122
10. 12355 - Twin Peaks, 501 Twin Peaks Blvd, San Francisco, CA 94131

#### PUSH Request- For creation of new trip
```
curl -H "Content-Type: application/json" -X POST -d '{"starting_from_location_id": "12348","location_ids" : [ "12350", "12346", "12347","12349"]}' http://localhost:8080/trips
```
Following will be the response for the above request:
```
{"Id":1122,"Status":"planning","Starting_from_location_id":"12348","Best_route_location_ids":["12349","12347","12350","12346"],"Total_uber_costs":46,"Total_uber_duration":3054,"Total_distance":26.610000000000003}
```

#### GET Request- For getting trip details of a specific trip_id
```
curl http://localhost:8080/trips/1122
```
Following will be the response for the above request:
```
{"Id":1122,"Status":"planning","Starting_from_location_id":"12348","Best_route_location_ids":["12349","12347","12350","12346"],"Total_uber_costs":46,"Total_uber_duration":3054,"Total_distance":26.610000000000003}

```

#### PUT Request- For updating status of the specific request_id
```
curl -H "Content-Type: application/json" -X PUT http://localhost:8080/trips/1122/request
```
Following will be the response for the above request:
```
{"Id":1122,"Status":"requesting","Starting_from_location_id":"12348","Next_destination_location_id":"12349","Best_route_location_ids":["12349","12347","12350","12346"],"Total_uber_costs":46,"Total_uber_duration":3054,"Total_distance":26.610000000000003,"Uber_wait_time_eta":6}
```

#### To add new locations to the database, use the following command
```
curl -H "Content-Type: application/json" -X POST -d '{"name" : "John Smith","address":"123 Main St","city": "San Francisco","state": "CA","zip":"94113"}' http://localhost:8080/locations
```
Following will be the response for the above request:
```
{"Id":12345,"Name":"John Smith","Address":"123 Main St","City":"San Francisco","State":"CA","Zip":"94113","Coordinate":{"Lat":37.7917618,"Lng":-122.3943405}}
```
