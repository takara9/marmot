package marmotd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/labstack/echo/v4"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	"github.com/takara9/marmot/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("Auth API handlers", Ordered, func() {
	const (
		port      = "26379"
		etcdImage = "ghcr.io/takara9/etcd:3.6.5"
	)

	var (
		containerID string
		d           *db.Database
		s           *marmotd.Server
		e           *echo.Echo
	)

	newContext := func(method, path, body, token string) (echo.Context, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		return e.NewContext(req, rec), rec
	}

	login := func(userID, password string) api.AuthLoginResponse {
		ctx, rec := newContext(http.MethodPost, "/auth/login", fmt.Sprintf(`{"userId":"%s","password":"%s"}`, userID, password), "")
		err := s.ApiAuthLogin(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusOK), rec.Body.String())

		var resp api.AuthLoginResponse
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		Expect(strings.TrimSpace(resp.AccessToken)).NotTo(BeEmpty())
		return resp
	}

	BeforeAll(func(ctx0 SpecContext) {
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%s:2379", port), etcdImage)
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("failed to start etcd container: %s (%v)", string(output), err))
		}
		containerID = string(output[:12])
		time.Sleep(10 * time.Second)

		url := fmt.Sprintf("http://127.0.0.1:%s", port)
		d, err = db.NewDatabase(url)
		Expect(err).NotTo(HaveOccurred())
		s = &marmotd.Server{Ma: &marmotd.Marmot{NodeName: "hvc", EtcdUrl: url, Db: d}}
		e = echo.New()

		hash, err := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
		Expect(err).NotTo(HaveOccurred())
		_, err = d.CreateUser(api.User{
			ApiVersion: "v1",
			Kind:       "User",
			Metadata: api.Metadata{Id: "admin", Name: "admin"},
			Spec: api.UserSpec{
				Enabled:            true,
				PasswordHash:       util.StringPtr(string(hash)),
				Roles:              &[]string{"Administrator"},
				MustChangePassword: util.BoolPtr(true),
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func(ctx0 SpecContext) {
		if d != nil {
			_ = d.Close()
		}
		if strings.TrimSpace(containerID) != "" {
			_, _ = exec.Command("docker", "stop", containerID).CombinedOutput()
		}
	})

	It("supports login, me, and logout flow", func() {
		loginResp := login("admin", "passw0rd")
		Expect(loginResp.User).NotTo(BeNil())
		Expect(loginResp.User.Spec.PasswordHash).To(BeNil())
		token := loginResp.AccessToken

		meCtx, meRec := newContext(http.MethodGet, "/auth/me", "", token)
		err := s.ApiAuthMe(meCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(meRec.Code).To(Equal(http.StatusOK), meRec.Body.String())

		var me api.AuthMe
		Expect(json.Unmarshal(meRec.Body.Bytes(), &me)).To(Succeed())
		Expect(me.UserId).To(Equal("admin"))
		Expect(me.Roles).To(ContainElement("Administrator"))

		logoutCtx, logoutRec := newContext(http.MethodPost, "/auth/logout", "", token)
		err = s.ApiAuthLogout(logoutCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(logoutRec.Code).To(Equal(http.StatusNoContent))

		meCtx2, meRec2 := newContext(http.MethodGet, "/auth/me", "", token)
		err = s.ApiAuthMe(meCtx2)
		Expect(err).NotTo(HaveOccurred())
		Expect(meRec2.Code).To(Equal(http.StatusUnauthorized))
	})

	It("returns uniform unauthorized for unknown user and bad password", func() {
		unknownCtx, unknownRec := newContext(http.MethodPost, "/auth/login", `{"userId":"nouser","password":"passw0rd"}`, "")
		err := s.ApiAuthLogin(unknownCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(unknownRec.Code).To(Equal(http.StatusUnauthorized), unknownRec.Body.String())

		badPassCtx, badPassRec := newContext(http.MethodPost, "/auth/login", `{"userId":"admin","password":"wrongpass"}`, "")
		err = s.ApiAuthLogin(badPassCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(badPassRec.Code).To(Equal(http.StatusUnauthorized), badPassRec.Body.String())
	})

	It("supports user CRUD, role assignment, and password policy", func() {
		token := login("admin", "passw0rd").AccessToken

		hash, err := bcrypt.GenerateFromPassword([]byte("userpass1"), bcrypt.DefaultCost)
		Expect(err).NotTo(HaveOccurred())
		createBody := fmt.Sprintf(`{"apiVersion":"v1","kind":"User","metadata":{"id":"alice","name":"alice"},"spec":{"enabled":true,"passwordHash":%q}}`, string(hash))
		createCtx, createRec := newContext(http.MethodPost, "/users", createBody, token)
		err = s.ApiCreateUser(createCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(createRec.Code).To(Equal(http.StatusCreated), createRec.Body.String())

		addRoleCtx, addRoleRec := newContext(http.MethodPost, "/users/alice/roles", `{"roleName":"Viewer"}`, token)
		err = s.ApiAddUserRole(addRoleCtx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(addRoleRec.Code).To(Equal(http.StatusNoContent), addRoleRec.Body.String())

		listRoleCtx, listRoleRec := newContext(http.MethodGet, "/users/alice/roles", "", token)
		err = s.ApiListUserRoles(listRoleCtx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(listRoleRec.Code).To(Equal(http.StatusOK), listRoleRec.Body.String())
		var roles api.RoleNames
		Expect(json.Unmarshal(listRoleRec.Body.Bytes(), &roles)).To(Succeed())
		Expect(roles).To(ContainElement("Viewer"))

		badPassCtx, badPassRec := newContext(http.MethodPost, "/users/alice/password", `{"newPassword":"abc!"}`, token)
		err = s.ApiChangeUserPassword(badPassCtx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(badPassRec.Code).To(Equal(http.StatusBadRequest), badPassRec.Body.String())

		setPassCtx, setPassRec := newContext(http.MethodPost, "/users/alice/password", `{"newPassword":"newpass01"}`, token)
		err = s.ApiChangeUserPassword(setPassCtx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(setPassRec.Code).To(Equal(http.StatusNoContent), setPassRec.Body.String())

		lockCtx, lockRec := newContext(http.MethodPost, "/users/alice/lock", "", token)
		err = s.ApiLockUserById(lockCtx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(lockRec.Code).To(Equal(http.StatusNoContent), lockRec.Body.String())

		unlockCtx, unlockRec := newContext(http.MethodPost, "/users/alice/unlock", "", token)
		err = s.ApiUnlockUserById(unlockCtx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(unlockRec.Code).To(Equal(http.StatusNoContent), unlockRec.Body.String())

		deleteCtx, deleteRec := newContext(http.MethodDelete, "/users/alice", "", token)
		err = s.ApiDeleteUserById(deleteCtx, "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(deleteRec.Code).To(Equal(http.StatusNoContent), deleteRec.Body.String())
	})

	It("denies user list to non-administrator roles", func() {
		viewerHash, err := bcrypt.GenerateFromPassword([]byte("viewerpass1"), bcrypt.DefaultCost)
		Expect(err).NotTo(HaveOccurred())
		_, err = d.CreateUser(api.User{
			ApiVersion: "v1",
			Kind:       "User",
			Metadata: api.Metadata{Id: "viewer2", Name: "viewer2"},
			Spec: api.UserSpec{
				Enabled:      true,
				PasswordHash: util.StringPtr(string(viewerHash)),
				Roles:        &[]string{"Viewer"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		viewerToken := login("viewer2", "viewerpass1").AccessToken

		router := echo.New()
		marmotd.RegisterRoutes(router, s, "/api/v1")

		viewerReq := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		viewerReq.Header.Set("Authorization", "Bearer "+viewerToken)
		viewerRec := httptest.NewRecorder()
		router.ServeHTTP(viewerRec, viewerReq)
		Expect(viewerRec.Code).To(Equal(http.StatusForbidden), viewerRec.Body.String())

		adminToken := login("admin", "passw0rd").AccessToken
		adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		adminReq.Header.Set("Authorization", "Bearer "+adminToken)
		adminRec := httptest.NewRecorder()
		router.ServeHTTP(adminRec, adminReq)
		Expect(adminRec.Code).To(Equal(http.StatusOK), adminRec.Body.String())

		cleanupReq := httptest.NewRequest(http.MethodDelete, "/api/v1/users/viewer2", nil)
		cleanupReq.Header.Set("Authorization", "Bearer "+adminToken)
		cleanupRec := httptest.NewRecorder()
		router.ServeHTTP(cleanupRec, cleanupReq)
		Expect(cleanupRec.Code).To(Equal(http.StatusNoContent), cleanupRec.Body.String())
	})

	It("supports API key create/list/delete", func() {
		token := login("admin", "passw0rd").AccessToken

		createCtx, createRec := newContext(http.MethodPost, "/users/admin/apikeys", `{}`, token)
		err := s.ApiCreateUserApiKey(createCtx, "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(createRec.Code).To(Equal(http.StatusCreated), createRec.Body.String())
		issued := strings.TrimSpace(createRec.Header().Get("X-Marmot-Api-Key"))
		Expect(issued).NotTo(BeEmpty())

		var key api.ApiKey
		Expect(json.Unmarshal(createRec.Body.Bytes(), &key)).To(Succeed())
		Expect(strings.TrimSpace(key.Metadata.Id)).NotTo(BeEmpty())

		listCtx, listRec := newContext(http.MethodGet, "/users/admin/apikeys", "", token)
		err = s.ApiListUserApiKeys(listCtx, "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(listRec.Code).To(Equal(http.StatusOK), listRec.Body.String())
		var keys api.ApiKeys
		Expect(json.Unmarshal(listRec.Body.Bytes(), &keys)).To(Succeed())
		Expect(len(keys)).To(BeNumerically(">=", 1))

		delCtx, delRec := newContext(http.MethodDelete, "/users/admin/apikeys/"+key.Metadata.Id, "", token)
		err = s.ApiDeleteUserApiKey(delCtx, "admin", key.Metadata.Id)
		Expect(err).NotTo(HaveOccurred())
		Expect(delRec.Code).To(Equal(http.StatusNoContent), delRec.Body.String())
	})

	It("enforces role-based access on registered routes", func() {
		adminToken := login("admin", "passw0rd").AccessToken

		viewerHash, err := bcrypt.GenerateFromPassword([]byte("viewerpass1"), bcrypt.DefaultCost)
		Expect(err).NotTo(HaveOccurred())
		_, err = d.CreateUser(api.User{
			ApiVersion: "v1",
			Kind:       "User",
			Metadata: api.Metadata{Id: "viewer1", Name: "viewer1"},
			Spec: api.UserSpec{
				Enabled:      true,
				PasswordHash: util.StringPtr(string(viewerHash)),
				Roles:        &[]string{"Viewer"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		viewerToken := login("viewer1", "viewerpass1").AccessToken

		router := echo.New()
		marmotd.RegisterRoutes(router, s, "/api/v1")

		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"apiVersion":"v1","kind":"User","metadata":{"id":"blocked","name":"blocked"},"spec":{"enabled":true,"passwordHash":"dummy"}}`))
		createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		createReq.Header.Set("Authorization", "Bearer "+viewerToken)
		createRec := httptest.NewRecorder()
		router.ServeHTTP(createRec, createReq)
		Expect(createRec.Code).To(Equal(http.StatusForbidden), createRec.Body.String())

		passReq := httptest.NewRequest(http.MethodPost, "/api/v1/users/viewer1/password", strings.NewReader(`{"newPassword":"viewerpass2"}`))
		passReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		passReq.Header.Set("Authorization", "Bearer "+viewerToken)
		passRec := httptest.NewRecorder()
		router.ServeHTTP(passRec, passReq)
		Expect(passRec.Code).To(Equal(http.StatusNoContent), passRec.Body.String())

		cleanupReq := httptest.NewRequest(http.MethodDelete, "/api/v1/users/viewer1", nil)
		cleanupReq.Header.Set("Authorization", "Bearer "+adminToken)
		cleanupRec := httptest.NewRecorder()
		router.ServeHTTP(cleanupRec, cleanupReq)
		Expect(cleanupRec.Code).To(Equal(http.StatusNoContent), cleanupRec.Body.String())
	})

	It("restricts authz check target to self or administrator", func() {
		adminToken := login("admin", "passw0rd").AccessToken

		viewerHash, err := bcrypt.GenerateFromPassword([]byte("viewerpass3"), bcrypt.DefaultCost)
		Expect(err).NotTo(HaveOccurred())
		_, err = d.CreateUser(api.User{
			ApiVersion: "v1",
			Kind:       "User",
			Metadata: api.Metadata{Id: "viewer3", Name: "viewer3"},
			Spec: api.UserSpec{
				Enabled:      true,
				PasswordHash: util.StringPtr(string(viewerHash)),
				Roles:        &[]string{"Viewer"},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		viewerToken := login("viewer3", "viewerpass3").AccessToken

		router := echo.New()
		marmotd.RegisterRoutes(router, s, "/api/v1")

		selfReq := httptest.NewRequest(http.MethodPost, "/api/v1/authz/check", strings.NewReader(`{"resource":"Server","action":"read"}`))
		selfReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		selfReq.Header.Set("Authorization", "Bearer "+viewerToken)
		selfRec := httptest.NewRecorder()
		router.ServeHTTP(selfRec, selfReq)
		Expect(selfRec.Code).To(Equal(http.StatusOK), selfRec.Body.String())

		otherReq := httptest.NewRequest(http.MethodPost, "/api/v1/authz/check", strings.NewReader(`{"userId":"admin","resource":"Server","action":"read"}`))
		otherReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		otherReq.Header.Set("Authorization", "Bearer "+viewerToken)
		otherRec := httptest.NewRecorder()
		router.ServeHTTP(otherRec, otherReq)
		Expect(otherRec.Code).To(Equal(http.StatusForbidden), otherRec.Body.String())

		adminReq := httptest.NewRequest(http.MethodPost, "/api/v1/authz/check", strings.NewReader(`{"userId":"viewer3","resource":"Server","action":"read"}`))
		adminReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		adminReq.Header.Set("Authorization", "Bearer "+adminToken)
		adminRec := httptest.NewRecorder()
		router.ServeHTTP(adminRec, adminReq)
		Expect(adminRec.Code).To(Equal(http.StatusOK), adminRec.Body.String())

		cleanupReq := httptest.NewRequest(http.MethodDelete, "/api/v1/users/viewer3", nil)
		cleanupReq.Header.Set("Authorization", "Bearer "+adminToken)
		cleanupRec := httptest.NewRecorder()
		router.ServeHTTP(cleanupRec, cleanupReq)
		Expect(cleanupRec.Code).To(Equal(http.StatusNoContent), cleanupRec.Body.String())
	})
})
