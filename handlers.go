package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/twinj/uuid"
	"gopkg.in/mgo.v2/bson"
)

const (
	E_INVALID_REQUEST           string = "invalid_request"
	E_UNAUTHORIZED_CLIENT              = "unauthorized_client"
	E_ACCESS_DENIED                    = "access_denied"
	E_UNSUPPORTED_RESPONSE_TYPE        = "unsupported_response_type"
	E_INVALID_SCOPE                    = "invalid_scope"
	E_SERVER_ERROR                     = "server_error"
	E_TEMPORARILY_UNAVAILABLE          = "temporarily_unavailable"
	E_UNSUPPORTED_GRANT_TYPE           = "unsupported_grant_type"
	E_INVALID_GRANT                    = "invalid_grant"
	E_INVALID_CLIENT                   = "invalid_client"
)

func HandlePublicInit(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("GET /init")

	hostnameKv := KV{}
	err := db.C("settings").Find(bson.M{"_id": "hostname"}).One(&hostnameKv)
	if err == nil {
		fmt.Println("Already active")
		return
	}

	var host string = r.Host
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}

	fmt.Println("Current hostname is ", host)

	var identity Identity = Identity{}
	identity.Hostname = host
	identity.Init()
	fmt.Println(identity)
	// selfJSON, err := json.Marshal(&identity)
	// if err == nil {

	_, err = db.C("identities").UpsertId(identity.ID, &identity)
	if err != nil {
		fmt.Println(err)
	}

	hostnameKv.Key = "hostname"
	hostnameKv.Value = host
	_, err = db.C("settings").UpsertId(hostnameKv.Key, &hostnameKv)
	if err != nil {
		fmt.Println(err)
	}

	rt.self = &identity
	w.WriteJson(rt.self)
	// }
}

func HandlePublicIndex(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("GET /")
	userData, ok := r.Env["REMOTE_USER"]
	if ok == true {
		w.WriteJson(userData)
	} else {
		w.WriteJson(rt.self)
	}
}

func HandlePublicIndexPost(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("POST /")

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}
	var identity Identity = Identity{}
	if err := json.Unmarshal(body, &identity); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		w.WriteJson("nop")
	} else {
		// rt.insertIdentity(&identity)
		_, err = db.C("identities").UpsertId(identity.ID, &identity)
		if err != nil {
			fmt.Println(err)
		}
		w.WriteJson(rt.self)
	}
}

func HandleOwnInstances(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("GET /instances")
	var instances []Instance
	err := db.C("instances").Find(bson.M{}).All(&instances)
	if err != nil {
		fmt.Println(err)
	}
	w.WriteJson(instances)
}

func HandleIdentityInstancesPost(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("POST /instances")

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}

	var instance Instance = Instance{}
	instance.SetPayloadFromJson(body)

	body, err = instance.ToJSON()
	if err == nil {
		fmt.Println(">>> GOT", string(body))
		valid, err := instance.Verify()
		if err == nil {
			if valid == true {
				fmt.Println(">>> IS VALID")
			} else {
				fmt.Println(">>> IS *NOT* VALID")
			}
		} else {
			fmt.Println(">>> error validating", err)
		}
	}
	w.WriteJson(rt.self)
}

func HandleOwnInstancesPost(w rest.ResponseWriter, r *rest.Request) {
	fmt.Println("POST /instances [OWN]")

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}

	var instance Instance = Instance{}
	instance.SetPayloadFromJson(body)
	instance.Owner = rt.self
	instance.ID = uuid.Formatter(uuid.NewV4(), uuid.CleanHyphen)
	instance.Payload.ID = instance.ID
	instance.Payload.Owner = instance.Owner.ID
	instance.Sign()

	_, err = db.C("instances").UpsertId(instance.ID, &instance)
	if err != nil {
		fmt.Println(err)
	}

	body, err = instance.ToJSON()
	if err == nil {
		fmt.Println(">>> GOT", string(body))
		// valid, err := instance.Verify()
		// if err == nil {
		// 	if valid == true {
		// 		fmt.Println(">>> IS VALID")
		// 	} else {
		// 		fmt.Println(">>> IS *NOT* VALID")
		// 	}
		// } else {
		// 	fmt.Println(">>> error validating", err)
		// }
	}
	w.WriteJson(body)
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

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}
	var identity Identity = Identity{}
	if err := json.Unmarshal(body, &identity); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		w.WriteJson("nop")
	} else {
		identity, err = FetchIdentity(identity.GetURI())
		// if err == nil {
		// 	rt.insertIdentity(&identity)
		// }
		_, err = db.C("identities").UpsertId(identity.ID, &identity)
		if err != nil {
			fmt.Println(err)
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
