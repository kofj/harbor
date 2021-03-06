package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/common/utils/log"
	"github.com/goharbor/harbor/src/core/api/models"
	"github.com/goharbor/harbor/src/replication/ng"
	"github.com/goharbor/harbor/src/replication/ng/model"
	"github.com/goharbor/harbor/src/replication/ng/registry"
)

// RegistryAPI handles requests to /api/registries/{}. It manages registries integrated to Harbor.
type RegistryAPI struct {
	BaseController
	manager registry.Manager
}

// Prepare validates the user
func (t *RegistryAPI) Prepare() {
	t.BaseController.Prepare()
	if !t.SecurityCtx.IsAuthenticated() {
		t.HandleUnauthorized()
		return
	}

	if !t.SecurityCtx.IsSysAdmin() {
		t.HandleForbidden(t.SecurityCtx.GetUsername())
		return
	}

	t.manager = ng.RegistryMgr
}

// Get gets a registry by id.
func (t *RegistryAPI) Get() {
	id := t.GetIDFromURL()

	registry, err := t.manager.Get(id)
	if err != nil {
		log.Errorf("failed to get registry %d: %v", id, err)
		t.CustomAbort(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	if registry == nil {
		t.HandleNotFound(fmt.Sprintf("registry %d not found", id))
		return
	}

	// Hide access secret
	registry.Credential.AccessSecret = "*****"

	t.Data["json"] = registry
	t.ServeJSON()
}

// List lists all registries that match a given registry name.
func (t *RegistryAPI) List() {
	name := t.GetString("name")

	_, registries, err := t.manager.List(&model.RegistryQuery{
		Name: name,
	})
	if err != nil {
		log.Errorf("failed to list registries %s: %v", name, err)
		t.CustomAbort(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	// Hide passwords
	for _, registry := range registries {
		registry.Credential.AccessSecret = "*****"
	}

	t.Data["json"] = registries
	t.ServeJSON()
	return
}

// Post creates a registry
func (t *RegistryAPI) Post() {
	registry := &model.Registry{}
	t.DecodeJSONReqAndValidate(registry)

	reg, err := t.manager.GetByName(registry.Name)
	if err != nil {
		log.Errorf("failed to get registry %s: %v", registry.Name, err)
		t.CustomAbort(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	if reg != nil {
		t.HandleConflict(fmt.Sprintf("name '%s' is already used", registry.Name))
		return
	}

	id, err := t.manager.Add(registry)
	if err != nil {
		log.Errorf("Add registry '%s' error: %v", registry.URL, err)
		t.CustomAbort(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	t.Redirect(http.StatusCreated, strconv.FormatInt(id, 10))
}

// Put updates a registry
func (t *RegistryAPI) Put() {
	id := t.GetIDFromURL()

	registry, err := t.manager.Get(id)
	if err != nil {
		log.Errorf("Get registry by id %d error: %v", id, err)
		t.CustomAbort(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}

	if registry == nil {
		t.HandleNotFound(fmt.Sprintf("Registry %d not found", id))
		return
	}

	req := models.RegistryUpdateRequest{}
	t.DecodeJSONReq(&req)

	originalName := registry.Name

	if req.Name != nil {
		registry.Name = *req.Name
	}
	if req.URL != nil {
		registry.URL = *req.URL
	}
	if req.CredentialType != nil {
		registry.Credential.Type = (model.CredentialType)(*req.CredentialType)
	}
	if req.AccessKey != nil {
		registry.Credential.AccessKey = *req.AccessKey
	}
	if req.AccessSecret != nil {
		registry.Credential.AccessSecret = *req.AccessSecret
	}
	if req.Insecure != nil {
		registry.Insecure = *req.Insecure
	}

	t.Validate(registry)

	if registry.Name != originalName {
		reg, err := t.manager.GetByName(registry.Name)
		if err != nil {
			log.Errorf("Get registry by name '%s' error: %v", registry.Name, err)
			t.CustomAbort(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
			return
		}

		if reg != nil {
			t.HandleConflict("name is already used")
			return
		}
	}

	if err := t.manager.Update(registry); err != nil {
		log.Errorf("Update registry %d error: %v", id, err)
		t.CustomAbort(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}
}

// Delete deletes a registry
func (t *RegistryAPI) Delete() {
	id := t.GetIDFromURL()

	registry, err := t.manager.Get(id)
	if err != nil {
		msg := fmt.Sprintf("Get registry %d error: %v", id, err)
		log.Error(msg)
		t.HandleInternalServerError(msg)
		return
	}

	if registry == nil {
		t.HandleNotFound(fmt.Sprintf("registry %d not found", id))
		return
	}

	// TODO: Use PolicyManager instead
	policies, err := dao.GetRepPolicyByTarget(id)
	if err != nil {
		msg := fmt.Sprintf("Get policies related to registry %d error: %v", id, err)
		log.Error(msg)
		t.HandleInternalServerError(msg)
		return
	}

	if len(policies) > 0 {
		msg := fmt.Sprintf("Can't delete registry with replication policies, %d found", len(policies))
		log.Error(msg)
		t.HandleStatusPreconditionFailed(msg)
		return
	}

	if err := t.manager.Remove(id); err != nil {
		msg := fmt.Sprintf("Delete registry %d error: %v", id, err)
		log.Error(msg)
		t.HandleInternalServerError(msg)
		return
	}
}
