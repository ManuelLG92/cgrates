//go:build integration
// +build integration

/*
Real-time Online/Offline Charging System (OCS) for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/
package general_tests

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/cgrates/birpc"
	"github.com/cgrates/birpc/context"
	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/engine"
	"github.com/cgrates/cgrates/utils"
)

func TestEscapeCharacters(t *testing.T) {

	// Set up config.
	content := `{
"general": {
	"log_level": 7,
},

"data_db": {
	"db_type": "*internal"
},

"stor_db": {
	"db_type": "*internal"
},

"attributes": {
	"enabled": true,
},

"apiers": {
	"enabled": true
}

}`
	cfg, cfgPath, clean, err := initTestCfg(content)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()

	// Flush datadb and stordb.
	err = engine.InitDataDb(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = engine.InitStorDb(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Start the engine.
	_, err = engine.StopStartEngine(cfgPath, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.KillEngine(1000)

	// Initialize a jsonrpc client.
	var client *birpc.Client
	client, err = newRPCClient(cfg.ListenCfg())
	if err != nil {
		t.Fatal("Could not connect to rater: ", err.Error())
	}

	/*
		When escape sequences are written manually, like \u0000 in the csv file, they are not interpreted as
		escape sequences but as literal characters. So, when they are read, what will be returned is the literal
		string \u0000 instead of the null character. The *req.Password field that would be set using the csv
		below will not match "password\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000" but will match
		"password\\u0000\\u0000\\u0000\\u0000\\u0000\\u0000\\u0000\\u0000" instead.

				#Tenant,ID,Contexts,FilterIDs,ActivationInterval,AttributeFilterIDs,Path,Type,Value,Blocker,Weight
				cgrates.org,ATTR_TP,*any,*string:~*req.Password:password\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000,,,,,,false,20
				cgrates.org,ATTR_TP,,,,,*req.Password,*constant,processed,,
	*/

	// One of the workarounds for the issue described above is to set the profile using the Set API:

	attrPrf := &engine.AttributeProfileWithAPIOpts{
		AttributeProfile: &engine.AttributeProfile{
			Tenant: "cgrates.org",
			ID:     "ATTR_ESCAPE",
			FilterIDs: []string{
				"*string:~*req.Password:password\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000"},
			Attributes: []*engine.Attribute{
				{
					Path:  utils.MetaReq + utils.NestingSep + "Password",
					Value: config.NewRSRParsersMustCompile("processed", utils.InfieldSep),
				},
			},
			Weight: 10,
		},
	}
	var reply string
	err = client.Call(context.Background(), utils.APIerSv1SetAttributeProfile, attrPrf, &reply)
	if err != nil {
		t.Fatal(err)
	}

	// Call AttributeSv1.ProcessEvent to check if filters match.
	ev := &utils.CGREvent{
		Tenant: "cgrates.org",
		Event: map[string]any{
			"Password": "password\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000",
		},
	}
	var rplyEv engine.AttrSProcessEventReply
	if err := client.Call(context.Background(), utils.AttributeSv1ProcessEvent,
		ev, &rplyEv); err != nil {
		t.Fatal(err)
	}
	if rplyEv.MatchedProfiles[0] != "cgrates.org:ATTR_ESCAPE" ||
		rplyEv.Event["Password"] != "processed" ||
		rplyEv.AlteredFields[0] != "*req.Password" {
		fmt.Println(rplyEv.MatchedProfiles[0] != "ATTR_ESCAPE")
		fmt.Println(rplyEv.Event["Password"] != "processed")
		fmt.Println(rplyEv.AlteredFields[0] != "*req.Password")
		t.Error("unexpected reply:", utils.ToJSON(rplyEv))
	}

}

func initTestCfg(cfgContent string) (*config.CGRConfig, string, func(), error) {
	folderNameSuffix, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return nil, "", nil, fmt.Errorf("could not generate random number for folder name suffix, err: %s", err.Error())
	}
	cfgPath := fmt.Sprintf("/tmp/config%d", folderNameSuffix)
	err = os.MkdirAll(cfgPath, 0755)
	if err != nil {
		return nil, "", nil, err
	}
	filePath := filepath.Join(cfgPath, "cgrates.json")
	err = os.WriteFile(filePath, []byte(cfgContent), 0644)
	if err != nil {
		os.RemoveAll(cfgPath)
		return nil, "", nil, err
	}
	var cfg *config.CGRConfig
	cfg, err = config.NewCGRConfigFromPath(cfgPath)
	if err != nil {
		os.RemoveAll(cfgPath)
		return nil, "", nil, err
	}
	removeFunc := func() {
		os.RemoveAll(cfgPath)
	}

	return cfg, cfgPath, removeFunc, nil
}