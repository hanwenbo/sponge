package generate

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zhufuyi/sponge/pkg/gofile"
	"github.com/zhufuyi/sponge/pkg/replacer"
	"github.com/zhufuyi/sponge/pkg/sql2code"
	"github.com/zhufuyi/sponge/pkg/sql2code/parser"
)

// ServiceAndHandlerCRUDCommand generate both service and handler CRUD code
func ServiceAndHandlerCRUDCommand() *cobra.Command {
	var (
		moduleName string // module name for go.mod
		serverName string // server name
		outPath    string // output directory
		dbTables   string // table names

		sqlArgs = sql2code.Args{
			Package:    "model",
			JSONTag:    true,
			GormType:   true,
			IsWebProto: true,
		}

		suitedMonoRepo bool // whether the generated code is suitable for mono-repo
	)

	cmd := &cobra.Command{
		Use:   "service-handler",
		Short: "Generate both grpc service and http handler CRUD code based on sql",
		Long: `generate both grpc service and http handler CRUD code based on sql.

Examples:
  # generate CRUD code.
  sponge micro service-handler --module-name=yourModuleName --server-name=yourServerName --db-driver=mysql --db-dsn=root:123456@(192.168.3.37:3306)/test --db-table=user

  # generate CRUD code with multiple table names.
  sponge micro service-handler --module-name=yourModuleName --server-name=yourServerName --db-driver=mysql --db-dsn=root:123456@(192.168.3.37:3306)/test --db-table=t1,t2

  # generate CRUD code and specify the server directory, Note: code generation will be canceled when the latest generated file already exists.
  sponge micro service-handler --db-driver=mysql --db-dsn=root:123456@(192.168.3.37:3306)/test --db-table=user --out=./yourServerDir

  # if you want the generated code to suited to mono-repo, you need to specify the parameter --suited-mono-repo=true
`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mdName, srvName, smr := getNamesFromOutDir(outPath)
			if mdName != "" {
				moduleName = mdName
				suitedMonoRepo = smr
			} else if moduleName == "" {
				return errors.New(`required flag(s) "module-name" not set, use "sponge micro service -h" for help`)
			}
			if srvName != "" {
				serverName = srvName
			} else if serverName == "" {
				return errors.New(`required flag(s) "server-name" not set, use "sponge micro service -h" for help`)
			}

			serverName = convertServerName(serverName)
			if suitedMonoRepo {
				outPath = changeOutPath(outPath, serverName)
			}
			if sqlArgs.DBDriver == DBDriverMongodb {
				sqlArgs.IsEmbed = false
			}

			tableNames := strings.Split(dbTables, ",")
			for _, tableName := range tableNames {
				if tableName == "" {
					continue
				}

				sqlArgs.DBTable = tableName
				codes, err := sql2code.Generate(&sqlArgs)
				if err != nil {
					return err
				}

				g := &serviceAndHandlerGenerator{
					moduleName: moduleName,
					serverName: serverName,
					dbDriver:   sqlArgs.DBDriver,
					isEmbed:    sqlArgs.IsEmbed,
					codes:      codes,
					outPath:    outPath,

					suitedMonoRepo: suitedMonoRepo,
				}
				outPath, err = g.generateCode()
				if err != nil {
					return err
				}
			}

			fmt.Printf(`
using help:
  1. move the folders "api" and "internal" to your project code folder.
  2. open a terminal and execute the command to generate code: make proto
  3. compile and run service: make run
  4. visit http://localhost:8080/apis/swagger/index.html in your browser, and test the http CRUD api.
     open the file "internal/service/xxx_client_test.go" using Goland or VS Code, and test the grpc CRUD api.

`)
			fmt.Printf("generate \"service-http\" code successfully, out = %s\n", outPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&moduleName, "module-name", "m", "", "module-name is the name of the module in the go.mod file")
	//_ = cmd.MarkFlagRequired("module-name")
	cmd.Flags().StringVarP(&serverName, "server-name", "s", "", "server name")
	//_ = cmd.MarkFlagRequired("server-name")
	cmd.Flags().StringVarP(&sqlArgs.DBDriver, "db-driver", "k", "mysql", "database driver, support mysql, mongodb, postgresql, tidb, sqlite")
	cmd.Flags().StringVarP(&sqlArgs.DBDsn, "db-dsn", "d", "", "database content address, e.g. user:password@(host:port)/database. Note: if db-driver=sqlite, db-dsn must be a local sqlite db file, e.g. --db-dsn=/tmp/sponge_sqlite.db") //nolint
	_ = cmd.MarkFlagRequired("db-dsn")
	cmd.Flags().StringVarP(&dbTables, "db-table", "t", "", "table name, multiple names separated by commas")
	_ = cmd.MarkFlagRequired("db-table")
	cmd.Flags().BoolVarP(&sqlArgs.IsEmbed, "embed", "e", false, "whether to embed gorm.model struct")
	cmd.Flags().BoolVarP(&suitedMonoRepo, "suited-mono-repo", "l", false, "whether the generated code is suitable for mono-repo")
	cmd.Flags().IntVarP(&sqlArgs.JSONNamedType, "json-name-type", "j", 1, "json tags name type, 0:snake case, 1:camel case")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "output directory, default is ./service_<time>,"+
		" if you specify the directory where the web or microservice generated by sponge, the module-name and server-name flag can be ignored")

	return cmd
}

type serviceAndHandlerGenerator struct {
	moduleName string
	serverName string
	dbDriver   string
	isEmbed    bool
	codes      map[string]string
	outPath    string

	suitedMonoRepo bool
}

// nolint
func (g *serviceAndHandlerGenerator) generateCode() (string, error) {
	subTplName := "service-handler"
	r := Replacers[TplNameSponge]
	if r == nil {
		return "", errors.New("replacer is nil")
	}

	if g.serverName == "" {
		g.serverName = g.moduleName
	}

	// setting up template information
	subDirs := []string{"internal/model", "internal/cache", "internal/dao",
		"internal/service", "internal/handler", "api/serverNameExample"} // only the specified subdirectory is processed, if empty or no subdirectory is specified, it means all files
	ignoreDirs := []string{} // specify the directory in the subdirectory where processing is ignored
	var ignoreFiles []string
	switch strings.ToLower(g.dbDriver) {
	case DBDriverMysql, DBDriverPostgresql, DBDriverTidb, DBDriverSqlite:
		ignoreFiles = []string{ // specify the files in the subdirectory to be ignored for processing
			"userExample.pb.go", "userExample.pb.validate.go", "userExample_grpc.pb.go", "userExample_router.pb.go", // api/serverNameExample/v1
			"init.go", "init_test.go", "init.go.mgo", // internal/model
			"doc.go", "cacheNameExample.go", "cacheNameExample_test.go", "cache/userExample.go.mgo", // internal/cache
			"dao/userExample.go.mgo",                                                                                                                                                                            // internal/dao
			"service.go", "service_test.go", "service/userExample_logic.go", "userExample_logic_test.go", "service/userExample_test.go", "service/userExample.go.mgo", "service/userExample_client_test.go.mgo", // internal/service
			"handler/userExample.go", "handler/userExample.go.mgo", "handler/userExample_test.go", "handler/userExample_logic_test.go", "handler/userExample_logic.go", "handler/userExample_logic.go.mgo", // internal/handler
		}
	case DBDriverMongodb:
		ignoreFiles = []string{ // specify the files in the subdirectory to be ignored for processing
			"userExample.pb.go", "userExample.pb.validate.go", "userExample_grpc.pb.go", "userExample_router.pb.go", // api/serverNameExample/v1
			"init.go", "init_test.go", "init.go.mgo", // internal/model
			"doc.go", "cacheNameExample.go", "cacheNameExample_test.go", "cache/userExample.go", "cache/userExample_test.go", // internal/cache
			"dao/userExample_test.go", "dao/userExample.go", // internal/dao
			"service.go", "service_test.go", "service/userExample_logic.go", "userExample_logic_test.go", "service/userExample_test.go", "service/userExample.go", "service/userExample_client_test.go", // internal/service
			"handler/userExample.go", "handler/userExample.go.mgo", "handler/userExample_test.go", "handler/userExample_logic_test.go", "handler/userExample_logic.go", "handler/userExample_logic.go.mgo", // internal/handler
		}
	default:
		return "", errors.New("unsupported db driver: " + g.dbDriver)
	}

	r.SetSubDirsAndFiles(subDirs)
	r.SetIgnoreSubDirs(ignoreDirs...)
	r.SetIgnoreSubFiles(ignoreFiles...)
	_ = r.SetOutputDir(g.outPath, subTplName)
	fields := g.addFields(r)
	r.SetReplacementFields(fields)
	if err := r.SaveFiles(); err != nil {
		return "", err
	}

	return r.GetOutputDir(), nil
}

func (g *serviceAndHandlerGenerator) addFields(r replacer.Replacer) []replacer.Field {
	var fields []replacer.Field

	fields = append(fields, deleteFieldsMark(r, modelFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, daoFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, daoMgoFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, daoTestFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, serviceLogicFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, protoFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, serviceClientFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, serviceClientMgoFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, serviceTestFile, startMark, endMark)...)
	fields = append(fields, []replacer.Field{
		{ // replace the contents of the model/userExample.go file
			Old: modelFileMark,
			New: g.codes[parser.CodeTypeModel],
		},
		{ // replace the contents of the dao/userExample.go file
			Old: daoFileMark,
			New: g.codes[parser.CodeTypeDAO],
		},
		{ // replace the contents of the handler/userExample_logic.go file
			Old: embedTimeMark,
			New: getEmbedTimeCode(g.isEmbed),
		},
		{ // replace the contents of the v1/userExample.proto file
			Old: protoFileMark,
			New: g.codes[parser.CodeTypeProto],
		},
		{ // replace the contents of the service/userExample_client_test.go file
			Old: serviceFileMark,
			New: adjustmentOfIDType(g.codes[parser.CodeTypeService], g.dbDriver),
		},
		{
			Old: selfPackageName + "/" + r.GetSourcePath(),
			New: g.moduleName,
		},
		{
			Old: "github.com/zhufuyi/sponge",
			New: g.moduleName,
		},
		// replace directory name
		{
			Old: strings.Join([]string{"api", "serverNameExample", "v1"}, gofile.GetPathDelimiter()),
			New: strings.Join([]string{"api", g.serverName, "v1"}, gofile.GetPathDelimiter()),
		},
		{
			Old: "api/serverNameExample/v1",
			New: fmt.Sprintf("api/%s/v1", g.serverName),
		},
		// Note: protobuf package no "-" signs allowed
		{
			Old: "api.serverNameExample.v1",
			New: fmt.Sprintf("api.%s.v1", g.serverName),
		},
		{
			Old: "userExampleNO       = 1",
			New: fmt.Sprintf("userExampleNO = %d", rand.Intn(99)+1),
		},
		{
			Old: "_userExampleNO       = 2",
			New: fmt.Sprintf("_userExampleNO       = %d", rand.Intn(99)+1),
		},
		{
			Old: g.moduleName + "/pkg",
			New: "github.com/zhufuyi/sponge/pkg",
		},
		{
			Old: "serverNameExample",
			New: g.serverName,
		},
		{
			Old: "userExample_client_test.go.mgo",
			New: "userExample_client_test.go",
		},
		{
			Old: "userExample_logic.go.mgo",
			New: "userExample.go",
		},
		{
			Old: "userExample.go.service",
			New: "userExample.go",
		},
		{
			Old: "userExample_logic.go",
			New: "userExample.go",
		},
		{
			Old: "userExample.go.mgo",
			New: "userExample.go",
		},
		{
			Old:             "UserExamplePb",
			New:             "UserExample",
			IsCaseSensitive: true,
		},
		{
			Old:             "UserExample",
			New:             g.codes[parser.TableName],
			IsCaseSensitive: true,
		},
	}...)

	if g.suitedMonoRepo {
		fs := SubServerCodeFields(r.GetOutputDir(), g.moduleName, g.serverName)
		fields = append(fields, fs...)
	}

	return fields
}
