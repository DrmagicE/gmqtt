# Contributing to Gmqtt
We welcome contributions to Gmqtt of any kind including documentation, plugin, test, bug reports, issues, feature requests, typo fix, etc.  

If you want to write some code, but don't know where to start or what you might want to do, take a look at the [Unplanned](https://github.com/DrmagicE/gmqtt/milestone/2) milestone.

## Contributing Code
Feel free submit a pull request. Any pull request must be related to one or more open issues.   
If you are submitting a complex feature, it is recommended to open up a discussion or design proposal on issue track to get feedback before you start.

### Code Style
Gmqtt is a Go project, it is recommended to follow the [CodeReviewComments](https://github.com/golang/go/wiki/CodeReviewComments) guidelines.  
When you’re ready to create a pull request, be sure to:
* Have unit test for the new code.
* Run [goimport](https://godoc.org/golang.org/x/tools/cmd/goimports).
* Run go test -race ./... 
* Build the project with race detection enable (go build -race .), and pass both V3 and V5 test cases (except test_flow_control2 [#68](https://github.com/eclipse/paho.mqtt.testing/issues/68)) in [paho.mqtt.testing](https://github.com/eclipse/paho.mqtt.testing/tree/master/interoperability)


### Testing
Testing is really important for building a robust application. Any new code or changes must come with unit test.

#### Mocking
Gmqtt uses [GoMock](https://github.com/golang/mock) to generate mock codes. The mock file must begin with the source file name and ends with `_mock.go`. For example, the following command will generate the mock file for `client.go`
```bash
mockgen -source=server/client.go -destination=/usr/local/gopath/src/github.com/DrmagicE/gmqtt/server/client_mock.go -package=server -self_package=github.com/DrmagicE/gmqtt/server
```
#### Assertion
Please use [testify](https://github.com/stretchr/testify) for easy assertion.

