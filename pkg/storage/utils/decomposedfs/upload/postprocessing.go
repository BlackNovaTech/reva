// Copyright 2018-2022 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package upload

import (
	"fmt"
	"time"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/cs3org/reva/v2/pkg/utils/postprocessing"
)

func configurePostprocessing(upload *Upload, o options.PostprocessingOptions) postprocessing.Postprocessing {
	waitfor := []string{"initialize"}
	if !o.AsyncFileUploads {
		waitfor = append(waitfor, "assembling")
	}

	steps := []postprocessing.Step{
		postprocessing.NewStep("initialize", func() error {
			// we need the node to start processing
			n, err := CreateNodeForUpload(upload)
			if err != nil {
				return err
			}

			// set processing status
			upload.node = n
			return upload.node.MarkProcessing()
		}, nil),
		postprocessing.NewStep("assembling", func() error {
			err := upload.finishUpload()
			// NOTE: this makes the testsuite happy - remove once adjusted
			if !o.AsyncFileUploads && upload.node != nil {
				_ = upload.node.UnmarkProcessing()
			}
			return err
		}, upload.cleanup, "initialize"),
	}
	if o.DelayProcessing != 0 {
		steps = append(steps, postprocessing.NewStep("sleep", func() error {
			time.Sleep(o.DelayProcessing)
			return nil
		}, nil))
	}

	return postprocessing.Postprocessing{
		Steps:   steps,
		WaitFor: waitfor,
		Finish: func(m map[string]error) {
			for alias, err := range m {
				if err != nil {
					upload.log.Info().Str("ID", upload.Info.ID).Str("step", alias).Err(err).Msg("postprocessing failed")
				}

			}

			if upload.node != nil {
				// unset processing status and propagate changes
				if err := upload.node.UnmarkProcessing(); err != nil {
					upload.log.Info().Str("path", upload.node.InternalPath()).Err(err).Msg("unmarking processing failed")
				}

				if o.AsyncFileUploads { // updating the mtime will cause the testsuite to fail - hence we do it only in async case
					now := utils.TSNow()
					if err := upload.node.SetMtime(upload.Ctx, fmt.Sprintf("%d.%d", now.Seconds, now.Nanos)); err != nil {
						upload.log.Info().Str("path", upload.node.InternalPath()).Err(err).Msg("could not set mtime")
					}
				}

				if err := upload.tp.Propagate(upload.Ctx, upload.node); err != nil {
					upload.log.Info().Str("path", upload.node.InternalPath()).Err(err).Msg("could not set mtime")
				}
			}
		},
	}
}
