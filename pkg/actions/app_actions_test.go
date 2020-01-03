package actions_test

import (
	. "github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppActions", func() {

	It("should assemble actions by reflection", func() {

		sut := &AppAction{
			HTTP: &HTTPAction{},
		}

		actions := sut.GetActions()
		Expect(actions).To(HaveLen(1))

		sut = &AppAction{
			HTTP:  &HTTPAction{},
			Bosun: &BosunAction{"test"},
		}

		actions = sut.GetActions()
		Expect(actions).To(HaveLen(2))

		scriptAction := ScriptAction("some script")
		sut = &AppAction{
			HTTP:   &HTTPAction{},
			Bosun:  &BosunAction{"test"},
			Script: &scriptAction,
		}

		actions = sut.GetActions()
		Expect(actions).To(HaveLen(3))

	})

	Describe("HTTPAction", func() {
		It("should execute request", func() {

			sut := HTTPAction{
				Method: "GET",
				URL:    "https://google.com",
			}

			ctx := NewTestActionContext()

			Expect(sut.Execute(ctx)).To(Succeed())
		})

		It("should execute raw request", func() {

			sut := HTTPAction{
				URL: "",
				Raw: `GET https://google.com HTTP/1.1
Host: google.com
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8
Accept-Encoding: gzip, deflate
Accept-Language: en-US,en;q=0.9

`,
			}

			ctx := NewTestActionContext()

			Expect(sut.Execute(ctx)).To(Succeed())
		})
	})

	Describe("MongoAction", func() {
		It("should unmarshal correctly", func() {

			raw := `
name: client-migration
when: BeforeDeploy
mongo:
  databaseFile: "test"
  connection:
    dbName: "auth"
    kubePort:
      forward: true
      port: 27017
      serviceName: mongodb-0
    credentials:
      type:       vault
      vaultPath:  database/creds/mongodb-provisioner
      authSource: admin
  command: {
      "find": "auth.clients"
  }
`
			var appAction AppAction
			Expect(yaml.Unmarshal([]byte(raw), &appAction)).To(Succeed())
			actual := appAction.Mongo
			Expect(actual).ToNot(BeNil())
			Expect(actual.DatabaseFile).To(Equal("test"))
			Expect(actual.Connection.Credentials.Type).To(Equal("vault"))
		})
	})
})
