/*
 * Copyright 2024 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package kloglevel

import (
	"flag"
	"fmt"

	"k8s.io/klog/v2"
)

func Get(cmdline *flag.FlagSet) (klog.Level, error) {
	level := getKLogLevel(cmdline)
	if level == nil {
		return 0, fmt.Errorf("cannot get the log level programmatically")
	}
	return *level, nil
}

func Set(cmdline *flag.FlagSet, v klog.Level) error {
	verbosity := fmt.Sprintf("%v", v)

	if level := getKLogLevel(cmdline); level != nil {
		return level.Set(verbosity)
	}

	// if modifying the flag value (which is recommended by klog) fails, then fallback to modifying
	// the internal state of klog using the empty new level.
	var newLevel klog.Level
	if err := newLevel.Set(verbosity); err != nil {
		return fmt.Errorf("failed set klog.logging.verbosity %s: %v", verbosity, err)
	}

	return nil
}

func getKLogLevel(cmdline *flag.FlagSet) *klog.Level {
	var level *klog.Level

	// First, if the '-v' was specified in command line, attempt to acquire the level pointer from it.
	if f := cmdline.Lookup("v"); f != nil {
		if flagValue, ok := f.Value.(*klog.Level); ok {
			level = flagValue
		}
	}

	if level != nil {
		return level
	}

	// Second, if the '-v' was not set but is still present in flags defined for the command, attempt to acquire it
	// by visiting all flags.
	cmdline.VisitAll(func(f *flag.Flag) {
		if level != nil {
			return
		}
		if levelFlag, ok := f.Value.(*klog.Level); ok {
			level = levelFlag
		}
	})

	return level
}
