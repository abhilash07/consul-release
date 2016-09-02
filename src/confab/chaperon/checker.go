package chaperon

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry-incubator/consul-release/src/confab/agent"
	"github.com/cloudfoundry-incubator/consul-release/src/confab/config"
	"github.com/cloudfoundry-incubator/consul-release/src/confab/utils"
	consulagent "github.com/hashicorp/consul/command/agent"
)

var (
	ioutilReadAll = ioutil.ReadAll
)

type BootstrapInput struct {
	AgentURL           string
	Logger             logger
	Controller         controller
	ConfigWriter       configWriter
	Config             config.Config
	GenerateRandomUUID RandomUUIDGenerator
	AgentRunner        agentRunner
	AgentClient        agentClient
	NewRPCClient       consulRPCClientConstructor
	Retrier            utils.Retrier
	Timeout            utils.Timeout
}

type RandomUUIDGenerator func(io.Reader) (string, error)

func StartInBootstrap(bootstrapInput BootstrapInput) (bool, error) {
	var rpcClient *consulagent.RPCClient

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.generate-random-uuid")
	randomID, err := bootstrapInput.GenerateRandomUUID(rand.Reader)
	if err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.generate-random-uuid.failed", err)
		return false, err
	}
	bootstrapInput.Config.Consul.Agent.Mode = "client"
	bootstrapInput.Config.Consul.Agent.NodeName = fmt.Sprintf("client-%s", randomID)

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.config-writer.write")
	err = bootstrapInput.ConfigWriter.Write(bootstrapInput.Config)
	if err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.config-writer.write.failed", err)
		return false, err
	}

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.agent-runner.run")
	err = bootstrapInput.AgentRunner.Run()
	if err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.agent-runner.run.failed", err)
		return false, err
	}
	defer func() {
		bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.controller.stop-agent")
		bootstrapInput.Controller.StopAgent(rpcClient)
	}()

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.waiting-for-agent")
	if err := bootstrapInput.Retrier.TryUntil(bootstrapInput.Timeout, bootstrapInput.AgentClient.Self); err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.waiting-for-agent.failed", err)
		return false, err
	}

	rpcClient, err = bootstrapInput.NewRPCClient("localhost:8400")
	if err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.creating-rpc-client.failed", err)
		return false, err
	}

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.agent-client.join-members")
	err = bootstrapInput.AgentClient.JoinMembers()
	switch err {
	case agent.NoMembersToJoinError:
		bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.agent-client.join-members.no-members-to-join")
		return true, nil
	case nil:
		break
	default:
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.agent-client.join-members.failed", err)
		return false, err
	}

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.agent-client.members")
	members, err := bootstrapInput.AgentClient.Members(false)
	if err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.agent-client.members.failed", err)
		return false, err
	}

	for _, member := range members {
		if member.Tags["bootstrap"] == "1" {
			bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.bootstrap-node-exists", lager.Data{"bootstrap-node": member.Name})
			return false, nil
		}
	}

	route := fmt.Sprintf("%s/v1/status/leader", bootstrapInput.AgentURL)
	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.http.get", lager.Data{"route": route})
	resp, err := http.Get(route)
	if err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.http.get.failed", err)
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		response, err := ioutilReadAll(resp.Body)
		if err != nil {
			err = fmt.Errorf("Leader check returned %d status: body could not be read %q", resp.StatusCode, err)
		} else {
			if strings.Contains(string(response), "No known Consul servers") && resp.StatusCode == http.StatusInternalServerError {
				return true, nil
			}

			err = fmt.Errorf("Leader check returned %d status with response %q", resp.StatusCode, string(response))
		}
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.http.get.invalid-response", err)
		return false, err
	}

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.json-decoder.decode")
	var leader string
	err = json.NewDecoder(resp.Body).Decode(&leader)
	if err != nil {
		bootstrapInput.Logger.Error("chaperon-checker.start-in-bootstrap.json-decoder.decode.failed", err)
		return false, err
	}

	if leader != "" {
		bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.leader-exists", lager.Data{"leader": leader})
		return false, nil
	}

	bootstrapInput.Logger.Info("chaperon-checker.start-in-bootstrap.bootstrap-true")
	return true, nil
}