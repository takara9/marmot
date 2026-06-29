package db_test

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("Auth", Ordered, func() {
	var port string = "22379"
	var url string = fmt.Sprintf("http://127.0.0.1:%s", port)
	var containerID string

	BeforeAll(func(ctx SpecContext) {
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%s:2379", port), "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12])
		fmt.Printf("Container started with ID: %s\n", containerID)
		time.Sleep(10 * time.Second)
	}, NodeTimeout(20*time.Second))

	AfterAll(func(ctx SpecContext) {
		if containerID == "" {
			return
		}
		fmt.Println("STOPPING CONTAINER:", containerID)
		cmd := exec.Command("docker", "stop", containerID)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
	}, NodeTimeout(20*time.Second))

	Describe("User, role, and API key persistence", func() {
		var d *db.Database
		var userID string
		var rawToken string

		BeforeAll(func() {
			var err error
			d, err = db.NewDatabase(url)
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a user and stores it", func() {
			hash, err := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			created, err := d.CreateUser(api.User{
				ApiVersion: "v1",
				Kind:       "User",
				Metadata: api.Metadata{
					Id:   "alice",
					Name: "alice",
				},
				Spec: api.UserSpec{
					Enabled:      true,
					PasswordHash: util.StringPtr(string(hash)),
					Comment:      util.StringPtr("test user"),
				},
			})
			Expect(err).NotTo(HaveOccurred())
			userID = created.Metadata.Id

			got, err := d.GetUserById(userID)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Metadata.Id).To(Equal("alice"))
			Expect(got.Spec.PasswordHash).NotTo(BeNil())

			users, err := d.ListUsers()
			Expect(err).NotTo(HaveOccurred())
			Expect(users).NotTo(BeEmpty())
		})

		It("updates a user comment without losing the password hash", func() {
			updatedComment := util.StringPtr("updated comment")
			err := d.UpdateUser(userID, api.User{
				Spec: api.UserSpec{
					Comment: updatedComment,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			got, err := d.GetUserById(userID)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Spec.Comment).NotTo(BeNil())
			Expect(*got.Spec.Comment).To(Equal("updated comment"))
			Expect(got.Spec.PasswordHash).NotTo(BeNil())
		})

		It("authenticates the user by password", func() {
			got, err := d.AuthenticateUser(userID, "passw0rd")
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Metadata.Id).To(Equal(userID))
			Expect(got.Status).NotTo(BeNil())
			Expect(got.Status.LastLoginAt).NotTo(BeNil())
		})

		It("manages roles and authorization", func() {
			roles, err := d.ListRoles()
			Expect(err).NotTo(HaveOccurred())
			Expect(roles).NotTo(BeEmpty())

			viewer, err := d.GetRoleByName("Viewer")
			Expect(err).NotTo(HaveOccurred())
			Expect(viewer.Metadata.Name).To(Equal("Viewer"))

			err = d.AddUserRole(userID, "Viewer")
			Expect(err).NotTo(HaveOccurred())

			assigned, err := d.ListUserRoles(userID)
			Expect(err).NotTo(HaveOccurred())
			Expect(assigned).To(ContainElement("Viewer"))

			allowed, err := d.Authorize(userID, "Server", "read")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())

			denied, err := d.Authorize(userID, "Server", "delete")
			Expect(err).NotTo(HaveOccurred())
			Expect(denied).To(BeFalse())

			err = d.DeleteUserRole(userID, "Viewer")
			Expect(err).NotTo(HaveOccurred())

			assigned, err = d.ListUserRoles(userID)
			Expect(err).NotTo(HaveOccurred())
			Expect(assigned).NotTo(ContainElement("Viewer"))
		})

		It("creates, resolves, and revokes API keys", func() {
			apiKey, token, err := d.CreateUserApiKey(userID, api.ApiKeyCreateRequest{
				Comment: util.StringPtr("cli access"),
			})
			Expect(err).NotTo(HaveOccurred())
			rawToken = token
			Expect(apiKey.Metadata.Id).NotTo(BeEmpty())

			keys, err := d.ListUserApiKeys(userID)
			Expect(err).NotTo(HaveOccurred())
			Expect(keys).To(HaveLen(1))
			Expect(keys[0].Spec.Comment).NotTo(BeNil())
			Expect(*keys[0].Spec.Comment).To(Equal("cli access"))

			resolvedUser, resolvedKey, err := d.AuthenticateApiKey(rawToken)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolvedUser.Metadata.Id).To(Equal(userID))
			Expect(resolvedKey.Metadata.Id).To(Equal(apiKey.Metadata.Id))

			err = d.DeleteUserApiKey(userID, apiKey.Metadata.Id)
			Expect(err).NotTo(HaveOccurred())

			keys, err = d.ListUserApiKeys(userID)
			Expect(err).NotTo(HaveOccurred())
			Expect(keys).To(BeEmpty())
			_, _, err = d.AuthenticateApiKey(rawToken)
			Expect(err).To(HaveOccurred())
		})

		It("physically cleans up revoked API keys after threshold", func() {
			targetUserID := userID
			if targetUserID == "" {
				targetUserID = "alice-cleanup"
			}

			if _, err := d.GetUserById(targetUserID); err != nil {
				hash, hashErr := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
				Expect(hashErr).NotTo(HaveOccurred())
				_, createErr := d.CreateUser(api.User{
					ApiVersion: "v1",
					Kind:       "User",
					Metadata: api.Metadata{
						Id:   targetUserID,
						Name: targetUserID,
					},
					Spec: api.UserSpec{
						Enabled:      true,
						PasswordHash: util.StringPtr(string(hash)),
					},
				})
				Expect(createErr).NotTo(HaveOccurred())
			}

			revokedKey, _, err := d.CreateUserApiKey(targetUserID, api.ApiKeyCreateRequest{
				Comment: util.StringPtr("revoked-for-cleanup"),
			})
			Expect(err).NotTo(HaveOccurred())

			activeKey, _, err := d.CreateUserApiKey(targetUserID, api.ApiKeyCreateRequest{
				Comment: util.StringPtr("active-for-cleanup"),
			})
			Expect(err).NotTo(HaveOccurred())

			err = d.DeleteUserApiKey(targetUserID, revokedKey.Metadata.Id)
			Expect(err).NotTo(HaveOccurred())

			deleted, err := d.CleanupRevokedApiKeysOlderThan(0)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted).To(BeNumerically(">=", 1))

			var removed api.ApiKey
			_, err = d.GetJSON("/marmot/user-apikey/"+targetUserID+"/"+revokedKey.Metadata.Id, &removed)
			Expect(err).To(HaveOccurred())

			var kept api.ApiKey
			_, err = d.GetJSON("/marmot/user-apikey/"+targetUserID+"/"+activeKey.Metadata.Id, &kept)
			Expect(err).NotTo(HaveOccurred())

			err = d.DeleteUserApiKey(targetUserID, activeKey.Metadata.Id)
			Expect(err).NotTo(HaveOccurred())
		})

		It("invalidates login session API key after 30 minutes of inactivity", func() {
			idleUserID := "idle-timeout-user"
			if _, err := d.GetUserById(idleUserID); err != nil {
				hash, hashErr := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
				Expect(hashErr).NotTo(HaveOccurred())
				_, createErr := d.CreateUser(api.User{
					ApiVersion: "v1",
					Kind:       "User",
					Metadata: api.Metadata{Id: idleUserID, Name: idleUserID},
					Spec: api.UserSpec{
						Enabled:      true,
						PasswordHash: util.StringPtr(string(hash)),
					},
				})
				Expect(createErr).NotTo(HaveOccurred())
			}

			sessionType := db.ApiKeySessionTypeLogin
			key, raw, err := d.CreateUserApiKey(idleUserID, api.ApiKeyCreateRequest{Comment: util.StringPtr("idle-timeout"), SessionType: &sessionType})
			Expect(err).NotTo(HaveOccurred())

			storageKey := "/marmot/user-apikey/" + idleUserID + "/" + key.Metadata.Id
			var rec api.ApiKey
			resp, err := d.GetJSON(storageKey, &rec)
			Expect(err).NotTo(HaveOccurred())
			if rec.Status == nil {
				rec.Status = &api.ApiKeyStatus{}
			}
			rec.Status.LastUsedAt = util.TimePtr(time.Now().Add(-31 * time.Minute))
			err = d.PutJSONCAS(storageKey, resp.Kvs[0].ModRevision, rec)
			Expect(err).NotTo(HaveOccurred())

			_, _, err = d.AuthenticateApiKey(raw)
			Expect(err).To(HaveOccurred())

			var after api.ApiKey
			_, err = d.GetJSON(storageKey, &after)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.Spec.Revoked).NotTo(BeNil())
			Expect(*after.Spec.Revoked).To(BeFalse())
			Expect(after.Status).NotTo(BeNil())
			Expect(after.Status.RevokedAt).To(BeNil())

			revoked, err := d.RevokeIdleLoginSessionsOlderThan(db.AuthSessionIdleTimeout)
			Expect(err).NotTo(HaveOccurred())
			Expect(revoked).To(BeNumerically(">=", 1))

			_, err = d.GetJSON(storageKey, &after)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.Spec.Revoked).NotTo(BeNil())
			Expect(*after.Spec.Revoked).To(BeTrue())
			Expect(after.Status).NotTo(BeNil())
			Expect(after.Status.RevokedAt).NotTo(BeNil())
		})

		It("keeps login session API key valid before 30 minutes of inactivity", func() {
			activeUserID := "idle-timeout-user-active"
			if _, err := d.GetUserById(activeUserID); err != nil {
				hash, hashErr := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
				Expect(hashErr).NotTo(HaveOccurred())
				_, createErr := d.CreateUser(api.User{
					ApiVersion: "v1",
					Kind:       "User",
					Metadata: api.Metadata{Id: activeUserID, Name: activeUserID},
					Spec: api.UserSpec{
						Enabled:      true,
						PasswordHash: util.StringPtr(string(hash)),
					},
				})
				Expect(createErr).NotTo(HaveOccurred())
			}

			sessionType := db.ApiKeySessionTypeLogin
			key, raw, err := d.CreateUserApiKey(activeUserID, api.ApiKeyCreateRequest{Comment: util.StringPtr("idle-active"), SessionType: &sessionType})
			Expect(err).NotTo(HaveOccurred())

			storageKey := "/marmot/user-apikey/" + activeUserID + "/" + key.Metadata.Id
			var rec api.ApiKey
			resp, err := d.GetJSON(storageKey, &rec)
			Expect(err).NotTo(HaveOccurred())
			if rec.Status == nil {
				rec.Status = &api.ApiKeyStatus{}
			}
			rec.Status.LastUsedAt = util.TimePtr(time.Now().Add(-29 * time.Minute))
			err = d.PutJSONCAS(storageKey, resp.Kvs[0].ModRevision, rec)
			Expect(err).NotTo(HaveOccurred())

			_, _, err = d.AuthenticateApiKey(raw)
			Expect(err).NotTo(HaveOccurred())

			var after api.ApiKey
			_, err = d.GetJSON(storageKey, &after)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.Spec.Revoked).NotTo(BeNil())
			Expect(*after.Spec.Revoked).To(BeFalse())
			Expect(after.Status).NotTo(BeNil())
			Expect(after.Status.RevokedAt).To(BeNil())
		})

		It("does not revoke generated API keys when they are idle", func() {
			persistentUserID := "idle-timeout-user-persistent"
			if _, err := d.GetUserById(persistentUserID); err != nil {
				hash, hashErr := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
				Expect(hashErr).NotTo(HaveOccurred())
				_, createErr := d.CreateUser(api.User{
					ApiVersion: "v1",
					Kind:       "User",
					Metadata: api.Metadata{Id: persistentUserID, Name: persistentUserID},
					Spec: api.UserSpec{
						Enabled:      true,
						PasswordHash: util.StringPtr(string(hash)),
					},
				})
				Expect(createErr).NotTo(HaveOccurred())
			}

			key, raw, err := d.CreateUserApiKey(persistentUserID, api.ApiKeyCreateRequest{Comment: util.StringPtr("generated-key")})
			Expect(err).NotTo(HaveOccurred())

			storageKey := "/marmot/user-apikey/" + persistentUserID + "/" + key.Metadata.Id
			var rec api.ApiKey
			resp, err := d.GetJSON(storageKey, &rec)
			Expect(err).NotTo(HaveOccurred())
			if rec.Status == nil {
				rec.Status = &api.ApiKeyStatus{}
			}
			rec.Status.LastUsedAt = util.TimePtr(time.Now().Add(-2 * time.Hour))
			err = d.PutJSONCAS(storageKey, resp.Kvs[0].ModRevision, rec)
			Expect(err).NotTo(HaveOccurred())

			_, _, err = d.AuthenticateApiKey(raw)
			Expect(err).NotTo(HaveOccurred())

			revoked, err := d.RevokeIdleLoginSessionsOlderThan(db.AuthSessionIdleTimeout)
			Expect(err).NotTo(HaveOccurred())
			Expect(revoked).To(BeZero())

			var after api.ApiKey
			_, err = d.GetJSON(storageKey, &after)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.Spec.Revoked).NotTo(BeNil())
			Expect(*after.Spec.Revoked).To(BeFalse())
			Expect(after.Status).NotTo(BeNil())
			Expect(after.Status.RevokedAt).To(BeNil())
		})

		It("locks and unlocks the user", func() {
			err := d.LockUserById(userID)
			Expect(err).NotTo(HaveOccurred())

			_, err = d.AuthenticateUser(userID, "passw0rd")
			Expect(err).To(MatchError(db.ErrUserLocked))

			err = d.UnlockUserById(userID)
			Expect(err).NotTo(HaveOccurred())

			_, err = d.AuthenticateUser(userID, "passw0rd")
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes the user", func() {
			err := d.DeleteUserById(userID)
			Expect(err).NotTo(HaveOccurred())

			_, err = d.GetUserById(userID)
			Expect(err).To(HaveOccurred())
		})

		It("preserves enabled=false on create", func() {
			hash, err := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			disabledID := "disabled-user"
			created, err := d.CreateUser(api.User{
				ApiVersion: "v1",
				Kind:       "User",
				Metadata: api.Metadata{
					Id:   disabledID,
					Name: disabledID,
				},
				Spec: api.UserSpec{
					Enabled:      false,
					PasswordHash: util.StringPtr(string(hash)),
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(created.Spec.Enabled).To(BeFalse())

			got, err := d.GetUserById(disabledID)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Spec.Enabled).To(BeFalse())

			err = d.DeleteUserById(disabledID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("SetUserPasswordHash initializes nil status safely", func() {
			hash, err := bcrypt.GenerateFromPassword([]byte("passw0rd"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			legacyID := "legacy-user"
			legacy := api.User{
				ApiVersion: "v1",
				Kind:       "User",
				Metadata: api.Metadata{
					Id:   legacyID,
					Name: legacyID,
				},
				Spec: api.UserSpec{
					Enabled:      true,
					PasswordHash: util.StringPtr(string(hash)),
				},
				Status: nil,
			}
			err = d.PutJSON("/marmot/user/"+legacyID, legacy)
			Expect(err).NotTo(HaveOccurred())

			newHash, err := bcrypt.GenerateFromPassword([]byte("newpassw0rd"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())
			err = d.SetUserPasswordHash(legacyID, string(newHash), util.BoolPtr(true))
			Expect(err).NotTo(HaveOccurred())

			updated, err := d.GetUserById(legacyID)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Status).NotTo(BeNil())
			Expect(updated.Status.PasswordUpdatedAt).NotTo(BeNil())
			Expect(updated.Spec.MustChangePassword).NotTo(BeNil())
			Expect(*updated.Spec.MustChangePassword).To(BeTrue())

			err = d.DeleteUserById(legacyID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("EnsureBootstrapAdmin seeds the default admin only on empty auth store", func() {
			users, err := d.ListUsers()
			Expect(err).NotTo(HaveOccurred())
			for _, u := range users {
				id := u.Metadata.Id
				if id == "" {
					id = u.Metadata.Name
				}
				if id == "" {
					continue
				}
				err = d.DeleteUserById(id)
				Expect(err).NotTo(HaveOccurred())
			}

			err = d.EnsureBootstrapAdmin()
			Expect(err).NotTo(HaveOccurred())

			admin, err := d.GetUserById(db.BootstrapAdminUserID)
			Expect(err).NotTo(HaveOccurred())
			Expect(admin.Spec.Enabled).To(BeTrue())
			Expect(admin.Spec.PasswordHash).NotTo(BeNil())
			Expect(admin.Spec.MustChangePassword).NotTo(BeNil())
			Expect(*admin.Spec.MustChangePassword).To(BeTrue())
			Expect(admin.Spec.Roles).NotTo(BeNil())
			Expect(*admin.Spec.Roles).To(ContainElement(db.BootstrapAdminRoleName))

			_, err = d.AuthenticateUser(db.BootstrapAdminUserID, db.BootstrapAdminPassword)
			Expect(err).NotTo(HaveOccurred())

			customHash, err := bcrypt.GenerateFromPassword([]byte("userpass1"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())
			_, err = d.CreateUser(api.User{
				ApiVersion: "v1",
				Kind:       "User",
				Metadata: api.Metadata{Id: "bob", Name: "bob"},
				Spec: api.UserSpec{
					Enabled:      true,
					PasswordHash: util.StringPtr(string(customHash)),
				},
			})
			Expect(err).NotTo(HaveOccurred())

			err = d.DeleteUserById(db.BootstrapAdminUserID)
			Expect(err).NotTo(HaveOccurred())

			err = d.EnsureBootstrapAdmin()
			Expect(err).NotTo(HaveOccurred())

			_, err = d.GetUserById(db.BootstrapAdminUserID)
			Expect(err).To(HaveOccurred())

			err = d.DeleteUserById("bob")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
