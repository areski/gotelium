package main

// Usage for countries
//
// curl -u admin:admin -i -d '{"Code":"FR","Name":"France"}' http://127.0.0.1:8080/countries
// curl -u admin:admin -i -d '{"Code":"US","Name":"United States"}' http://127.0.0.1:8080/countries
// curl -u admin:admin -i http://127.0.0.1:8080/countries/FR
// curl -u admin:admin -i http://127.0.0.1:8080/countries/US
// curl -u admin:admin -i http://127.0.0.1:8080/countries
// curl -u admin:admin -i -X DELETE http://127.0.0.1:8080/countries/FR
// curl -u admin:admin -i http://127.0.0.1:8080/countries
// curl -u admin:admin -i -X DELETE http://127.0.0.1:8080/countries/US
// curl -u admin:admin -i http://127.0.0.1:8080/countries
//
// Usage for reminders
//
// curl -u admin:admin -i -d '{"Message":"this is a test"}' http://127.0.0.1:8080/api/reminders
// curl -u admin:admin -i http://127.0.0.1:8080/api/reminders/1
// curl -u admin:admin -i http://127.0.0.1:8080/api/reminders
// curl -u admin:admin -i -X PUT -d '{"Message":"is updated"}' http://127.0.0.1:8080/api/reminders/1
// curl -u admin:admin -i -X DELETE http://127.0.0.1:8080/api/reminders/1

import (
	// "fmt"
	"github.com/ant0ine/go-json-rest/rest"
	// "github.com/stretchr/graceful"
	_ "github.com/mattn/go-sqlite3"
	// _ "github.com/lib/pq"
	"github.com/jinzhu/gorm"
	"log"
	"net/http"
	"sync"
	"time"
)

func main() {

	api := Api{}
	api.InitDB()
	api.InitSchema()

	handler := rest.ResourceHandler{
		EnableStatusService:      true,
		EnableRelaxedContentType: true,
		PreRoutingMiddlewares: []rest.Middleware{
			&rest.AuthBasicMiddleware{
				Realm: "test zone",
				Authenticator: func(userId string, password string) bool {
					if userId == "admin" && password == "admin" {
						return true
					}
					return false
				},
			},
		},
	}
	svmw := SemVerMiddleware{
		MinVersion: "1.2.0",
		MaxVersion: "3.0.0",
	}
	err := handler.SetRoutes(
		&rest.Route{"GET", "/#version/countries", svmw.MiddlewareFunc(GetAllCountries)},
		&rest.Route{"POST", "/countries", PostCountry},
		&rest.Route{"GET", "/countries/:code", GetCountry},
		&rest.Route{"DELETE", "/countries/:code", DeleteCountry},
		&rest.Route{"GET", "/.status",
			func(w rest.ResponseWriter, r *rest.Request) {
				w.WriteJson(handler.GetStatus())
			},
		},
		// ORM
		&rest.Route{"GET", "/reminders", api.GetAllReminders},
		&rest.Route{"POST", "/reminders", api.PostReminder},
		&rest.Route{"GET", "/reminders/:id", api.GetReminder},
		&rest.Route{"PUT", "/reminders/:id", api.PutReminder},
		&rest.Route{"DELETE", "/reminders/:id", api.DeleteReminder},
	)
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/api/", http.StripPrefix("/api", &handler))
	log.Fatal(http.ListenAndServe(":8080", nil))
	// log.Fatal(http.ListenAndServe(":8080", &handler))

}

type Country struct {
	Code string
	Name string
}

var store = map[string]*Country{}

var lock = sync.RWMutex{}

func GetCountry(w rest.ResponseWriter, r *rest.Request) {
	code := r.PathParam("code")

	lock.RLock()

	var country *Country
	if store[code] != nil {
		country = &Country{}
		*country = *store[code]
	}
	lock.RUnlock()
	if country != nil {
		rest.NotFound(w, r)
		return
	}
	w.WriteJson(country)
}

func GetAllCountries(w rest.ResponseWriter, r *rest.Request) {
	lock.RLock()
	countries := make([]Country, len(store))
	i := 0
	for _, country := range store {
		countries[i] = *country
		i++
	}
	lock.RUnlock()
	w.WriteJson(&countries)
}

func PostCountry(w rest.ResponseWriter, r *rest.Request) {
	country := Country{}
	err := r.DecodeJsonPayload(&country)
	if err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if country.Code == "" {
		rest.Error(w, "country code required", 400)
		return
	}
	if country.Name == "" {
		rest.Error(w, "country name required", 400)
		return
	}
	lock.Lock()
	store[country.Code] = &country
	lock.Unlock()
	w.WriteJson(&country)
}

func DeleteCountry(w rest.ResponseWriter, r *rest.Request) {
	code := r.PathParam("code")
	lock.Lock()
	delete(store, code)
	lock.Unlock()
	w.WriteHeader(http.StatusOK)
}

/********* ORM ************/

type Reminder struct {
	Id        int64     `json:"id"`
	Message   string    `sql:"size:1024" json:"message"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	DeletedAt time.Time `json:"-"`
}

type Api struct {
	DB gorm.DB
}

func (api *Api) InitDB() {
	var err error
	// api.DB, err = gorm.Open("postgres", "user=gorm dbname=gorm sslmode=disable")
	// api.DB, err = gorm.Open("mysql", "user:password@/dbname?charset=utf8&parseTime=True")
	api.DB, err = gorm.Open("sqlite3", "database.db")
	if err != nil {
		log.Fatalf("Got error when connect database, the error is '%v'", err)
	}
	api.DB.LogMode(true)
}

func (api *Api) InitSchema() {
	api.DB.AutoMigrate(&Reminder{})
}

func (api *Api) GetAllReminders(w rest.ResponseWriter, r *rest.Request) {
	reminders := []Reminder{}
	api.DB.Find(&reminders)
	w.WriteJson(&reminders)
}

func (api *Api) GetReminder(w rest.ResponseWriter, r *rest.Request) {
	id := r.PathParam("id")
	reminder := Reminder{}
	if api.DB.First(&reminder, id).Error != nil {
		rest.NotFound(w, r)
		return
	}
	w.WriteJson(&reminder)
}

func (api *Api) PostReminder(w rest.ResponseWriter, r *rest.Request) {
	reminder := Reminder{}
	if err := r.DecodeJsonPayload(&reminder); err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := api.DB.Save(&reminder).Error; err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteJson(&reminder)
}

func (api *Api) PutReminder(w rest.ResponseWriter, r *rest.Request) {

	id := r.PathParam("id")
	reminder := Reminder{}
	if api.DB.First(&reminder, id).Error != nil {
		rest.NotFound(w, r)
		return
	}

	updated := Reminder{}
	if err := r.DecodeJsonPayload(&updated); err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reminder.Message = updated.Message

	if err := api.DB.Save(&reminder).Error; err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteJson(&reminder)
}

func (api *Api) DeleteReminder(w rest.ResponseWriter, r *rest.Request) {
	id := r.PathParam("id")
	reminder := Reminder{}
	if api.DB.First(&reminder, id).Error != nil {
		rest.NotFound(w, r)
		return
	}
	if err := api.DB.Delete(&reminder).Error; err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
