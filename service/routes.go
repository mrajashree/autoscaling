package service

import (
	"github.com/gorilla/mux"
	// "github.com/rancher/go-rancher/api"
	// v1Client "github.com/rancher/go-rancher/client"
	"github.com/rancher/go-rancher/v2"
)

var schemas *client.Schemas

var router *mux.Router

func NewRouter() *mux.Router {
	schemas = &client.Schemas{}

	// ApiVersion
	apiVersion := schemas.AddType("apiVersion", client.Resource{})
	apiVersion.CollectionMethods = []string{}

	// Schema
	schemas.AddType("schema", client.Schema{})
	// API framework routes
	router = mux.NewRouter().StrictSlash(true)

	return router
}
