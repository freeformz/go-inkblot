package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/twinj/uuid"
	"gopkg.in/mgo.v2/bson"
)

func clientError(w rest.ResponseWriter, errorString string) {
	rest.Error(w, errorString, http.StatusBadRequest)
}

func internal(w rest.ResponseWriter, errorString string) {
	rest.Error(w, errorString, http.StatusInternalServerError)
}

func unauthorized(w rest.ResponseWriter) {
	rest.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

func unprocessable(w rest.ResponseWriter) {
	rest.Error(w, "Unprocessable", 422)
}

func HandlePublicRegisterPost(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("POST /setup")

	hostnameKv := KV{}
	err := db.C("settings").Find(bson.M{"_id": "hostname"}).One(&hostnameKv)
	if err == nil {
		unauthorized(w)
		return
	}

	type SetupData struct {
		Password  string `json:"password"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	}
	setupData := SetupData{}
	err = r.DecodeJsonPayload(&setupData)

	if err != nil {
		unprocessable(w)
		return
	}

	var host string = r.Host
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}

	fmt.Println("Current hostname is ", host)

	var identity Identity = Identity{}
	identity.Hostname = host
	identity.FirstName = setupData.FirstName
	identity.LastName = setupData.LastName

	h := sha1.New()
	h.Write([]byte(setupData.Password))
	passhash := h.Sum(nil)

	identity.Passhash = fmt.Sprintf("%x", passhash)
	identity.Init()
	fmt.Println(identity)
	// selfJSON, err := json.Marshal(&identity)
	// if err == nil {

	_, err = db.C("identities").UpsertId(identity.ID, &identity)
	if err != nil {
		fmt.Println(err)
		internal(w, "Could not insert identity")
		return
	}

	hostnameKv.Key = "hostname"
	hostnameKv.Value = host
	_, err = db.C("settings").UpsertId(hostnameKv.Key, &hostnameKv)
	if err != nil {
		fmt.Println(err)
		internal(w, "Could not insert setting")
		return
	}

	rt.self = &identity
	w.WriteJson(rt.self)
	// }
}

func HandlePublicIndex(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("GET /")
	_, ok := r.Env["REMOTE_USER"]
	if ok == true {
		w.WriteJson(rt.self)
	} else {
		w.WriteJson(rt.self)
	}
}

func HandlePublicIndexPost(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("POST /")

	var identity Identity = Identity{}
	err := r.DecodeJsonPayload(&identity)
	if err != nil {
		unprocessable(w)
	} else {
		// rt.insertIdentity(&identity)
		_, err = db.C("identities").UpsertId(identity.ID, &identity)
		if err != nil {
			fmt.Println(err)
		}
		w.WriteJson(rt.self)
	}
}

func HandleInstances(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("GET /instances")
	var instances []Instance = make([]Instance, 0)
	var instancesJson []Payload = make([]Payload, 0)
	err := db.C("instances").Find(bson.M{}).All(&instances)
	if err == nil {
		for _, instance := range instances {
			instancesJson = append(instancesJson, instance.Payload)
		}
		w.WriteJson(instancesJson)
		return
	}
	internal(w, err.Error())
}

func HandleInstancesPost(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("POST /instances")

	// Get body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}

	// Create and populate instance
	var instance Instance = Instance{}
	instance.SetPayloadFromJson(body)

	// Check if request is authorized
	_, ok := r.Env["REMOTE_USER"]
	if ok == true {
		// Request is from our own identity
		instance.Owner = rt.self
		instance.ID = uuid.Formatter(uuid.NewV4(), uuid.CleanHyphen)
		instance.Payload.ID = instance.ID
		instance.Payload.Owner = instance.Owner.ID
		err = instance.Sign()

		if err == nil {
			// Insert instance
			_, err = db.C("instances").UpsertId(instance.ID, &instance)
			if err == nil {
				w.WriteJson(instance.Payload)
				instance.Push()
				instance.Broadcast()
				return
			}
		}
	} else {
		// Request is not authorized
		// so check if it should be
		if instance.Payload.Owner == rt.self.ID {
			fmt.Println("Missing authentication")
			clientError(w, "An authenticated request is required to post as this identity's owner")
			return
		}
		// Request is from a different identity
		valid, err := instance.Verify()
		if err == nil {
			if valid == true {
				fmt.Println(">>> IS VALID")
				// Insert instance
				_, err = db.C("instances").UpsertId(instance.ID, &instance)
				if err == nil {
					w.WriteJson(instance.Payload)
					instance.Push()
					instance.Broadcast()
					return
				}
			} else {
				fmt.Println(">>> IS *NOT* VALID")
				clientError(w, "Invalid signature")
				return
			}
		} else {
			fmt.Println(">>> error validating", err)
			clientError(w, "Could not validate signature")
			return
		}
	}
	clientError(w, err.Error())
}

func HandleOwnIdentities(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("GET /identities")
	var identities []Identity
	err := db.C("identities").Find(bson.M{}).All(&identities)
	if err != nil {
		fmt.Println(err)
	}
	w.WriteJson(identities)
}

func HandleOwnIdentitiesPost(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("POST /identities")

	var identity Identity = Identity{}
	err := r.DecodeJsonPayload(&identity)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		w.WriteJson("nop")
	} else {
		if identity.Hostname == "" {
			clientError(w, "Missing hostname")
			return
		}
		identity, err = FetchIdentity(identity.GetURI())
		fmt.Println("ER", err)
		if err != nil {
			clientError(w, "Could not retrieve identity")
			return
		}
		_, err = db.C("identities").UpsertId(identity.ID, &identity)
		if err != nil {
			fmt.Println(err)
			internal(w, "Could not upsert identity")
			return
		}
		w.WriteJson(identity)
	}
}

func HandleOwnSettings(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("GET /settings")
	var settings []KV
	err := db.C("settings").Find(bson.M{}).All(&settings)
	if err != nil {
		fmt.Println(err)
	}
	w.WriteJson(settings)
}
