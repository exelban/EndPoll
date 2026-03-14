package types

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig_NewConfig(t *testing.T) {
	t.Run("create new config", func(t *testing.T) {
		fileName := "/tmp/test.yaml"
		defer func() {
			_ = os.Remove(fileName)
		}()
		ctx := context.Background()

		_, err := os.Stat(fileName)
		require.Error(t, err)

		cfg, err := NewConfig(ctx, fileName)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		_, err = os.Stat(fileName)
		require.NoError(t, err)
	})
	t.Run("watch for file changes", func(t *testing.T) {
		file, _ := os.CreateTemp("/tmp", "*.json")
		defer os.Remove(file.Name())

		_, err := file.Write([]byte(`{}`))
		require.NoError(t, err)

		ctx := context.Background()
		cfg, err := NewConfig(ctx, file.Name())
		require.NoError(t, err)
		require.NotNil(t, cfg)

		wait := make(chan bool)
		go func() {
			<-cfg.FW
			wait <- true
		}()

		_, err = file.Write([]byte(`{"hosts": [{"url": "test"}]}`))
		require.NoError(t, err)

		<-wait
	})
}

func TestConfig_Parse(t *testing.T) {
	yamlFile, _ := os.CreateTemp("/tmp", "*.yaml")
	jsonFile, _ := os.CreateTemp("/tmp", "*.json")
	txtFile, _ := os.CreateTemp("/tmp", "*.txt")

	defer func() {
		_ = os.Remove(yamlFile.Name())
		_ = os.Remove(jsonFile.Name())
		_ = os.Remove(txtFile.Name())
	}()

	_, err := jsonFile.Write([]byte(`{}`))
	require.NoError(t, err)

	t.Run("no file", func(t *testing.T) {
		cfg := &Cfg{
			path: "no-file",
		}
		require.Error(t, cfg.Parse())
	})

	t.Run("wrong file format", func(t *testing.T) {
		cfg := &Cfg{
			path: txtFile.Name(),
		}
		err := cfg.Parse()
		require.EqualError(t, err, fmt.Sprintf("unknown configuration format `%s`", txtFile.Name()))
	})

	t.Run("json config", func(t *testing.T) {
		cfg := &Cfg{
			path: jsonFile.Name(),
		}
		require.NoError(t, cfg.Parse())
	})
	t.Run("yaml config", func(t *testing.T) {
		cfg := &Cfg{
			path: yamlFile.Name(),
		}
		require.NoError(t, cfg.Parse())
	})
}

func TestConfig_Validate(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		cfg := &Cfg{}
		err := cfg.Validate()
		require.EqualError(t, err, "no hosts for monitoring")
	})
	t.Run("host without URL", func(t *testing.T) {
		cfg := &Cfg{
			FileHosts: []*Host{
				{},
			},
		}
		err := cfg.Validate()
		require.EqualError(t, err, "host cannot be without url")
	})

	t.Run("parameters", func(t *testing.T) {
		t.Run("intervals", func(t *testing.T) {
			t.Run("retry", func(t *testing.T) {
				cfg := &Cfg{
					Interval: 123 * time.Second,
					Conditions: &Success{
						Code: []int{200},
					},
					FileHosts: []*Host{
						{
							URL: "test",
						},
					},
				}

				require.NoError(t, cfg.Validate())
				require.NotNil(t, cfg.Hosts[0].Interval)
				require.Equal(t, time.Second*123, *cfg.Hosts[0].Interval)
			})
			t.Run("timeout", func(t *testing.T) {
				cfg := &Cfg{
					Interval: 1 * time.Second,
					Timeout:  123 * time.Second,
					Conditions: &Success{
						Code: []int{200},
					},
					FileHosts: []*Host{
						{
							URL: "test",
						},
					},
				}

				require.NoError(t, cfg.Validate())
				require.NotNil(t, cfg.Hosts[0].TimeoutInterval)
				require.Equal(t, time.Second*123, *cfg.Hosts[0].TimeoutInterval)
			})
			t.Run("initial delay", func(t *testing.T) {
				delay := 123 * time.Second
				cfg := &Cfg{
					Interval:     time.Second,
					Timeout:      time.Second,
					InitialDelay: &delay,
					Conditions: &Success{
						Code: []int{200},
					},
					FileHosts: []*Host{
						{
							URL: "test",
						},
					},
				}

				require.NoError(t, cfg.Validate())

				require.NotNil(t, cfg.Hosts[0].InitialDelay)
				require.Equal(t, delay, *cfg.Hosts[0].InitialDelay)
			})

			t.Run("default when all empty", func(t *testing.T) {
				cfg := &Cfg{
					FileHosts: []*Host{
						{
							URL: "test",
						},
					},
				}

				require.NoError(t, cfg.Validate())

				require.Nil(t, cfg.Hosts[0].InitialDelay)

				require.NotNil(t, cfg.Hosts[0].Interval)
				require.Equal(t, 30*time.Second, *cfg.Hosts[0].Interval)

				require.NotNil(t, cfg.Hosts[0].TimeoutInterval)
				require.Equal(t, 60*time.Second, *cfg.Hosts[0].TimeoutInterval)
			})
		})
		t.Run("success code", func(t *testing.T) {
			t.Run("default values", func(t *testing.T) {
				cfg := &Cfg{
					FileHosts: []*Host{
						{
							URL: "ok",
						},
					},
				}
				require.NoError(t, cfg.Validate())
				require.Equal(t, []int{200, 201, 202, 203, 204, 205, 206, 207, 208}, cfg.Hosts[0].Conditions.Code)
			})
			t.Run("custom", func(t *testing.T) {
				cfg := &Cfg{
					Conditions: &Success{
						Code: []int{500},
					},
					FileHosts: []*Host{
						{
							URL: "ok",
						},
					},
				}
				require.NoError(t, cfg.Validate())
				require.Equal(t, []int{500}, cfg.Hosts[0].Conditions.Code)
			})
		})
		t.Run("thresholds", func(t *testing.T) {
			t.Run("default values", func(t *testing.T) {
				cfg := &Cfg{
					FileHosts: []*Host{
						{
							URL: "ok",
						},
					},
				}
				require.NoError(t, cfg.Validate())
				require.NotNil(t, cfg.Hosts[0].SuccessThreshold)
				require.Equal(t, 1, cfg.Hosts[0].SuccessThreshold)

				require.NotNil(t, cfg.Hosts[0].FailureThreshold)
				require.Equal(t, 2, cfg.Hosts[0].FailureThreshold)
			})
			t.Run("custom", func(t *testing.T) {
				cfg := &Cfg{
					SuccessThreshold: 500,
					FailureThreshold: 501,
					FileHosts: []*Host{
						{
							URL: "ok",
						},
					},
				}
				require.NoError(t, cfg.Validate())
				require.NotNil(t, cfg.Hosts[0].SuccessThreshold)
				require.Equal(t, 500, cfg.Hosts[0].SuccessThreshold)
				require.NotNil(t, cfg.Hosts[0].FailureThreshold)
				require.Equal(t, 501, cfg.Hosts[0].FailureThreshold)
			})
		})
		t.Run("conditions", func(t *testing.T) {
			cfg := &Cfg{
				Conditions: &Success{
					Code: []int{200},
				},
				Headers: map[string]string{
					"key-1": "value-1",
					"key-2": "value-2",
					"key-3": "value-3",
				},
				FileHosts: []*Host{
					{
						URL: "ok",
						Headers: map[string]string{
							"key-2": "value-2-2",
						},
					},
				},
			}

			require.NoError(t, cfg.Validate())

			require.Len(t, cfg.Hosts[0].Headers, 3)

			require.Equal(t, "value-1", cfg.Hosts[0].Headers["key-1"])
			require.Equal(t, "value-2-2", cfg.Hosts[0].Headers["key-2"])
			require.Equal(t, "value-3", cfg.Hosts[0].Headers["key-3"])
		})
	})

	t.Run("add host", func(t *testing.T) {
		cfg := &Cfg{
			FileHosts: []*Host{
				{URL: "test"},
			},
		}
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 1)
		require.Equal(t, "test", cfg.Hosts[0].URL)

		cfg.FileHosts = append(cfg.FileHosts, &Host{URL: "new"})
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 2)
		require.Equal(t, "new", cfg.Hosts[1].URL)
	})
	t.Run("remove host", func(t *testing.T) {
		cfg := &Cfg{
			FileHosts: []*Host{
				{URL: "test-1"},
				{URL: "test-2"},
			},
		}
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 2)

		cfg.FileHosts = cfg.FileHosts[:1]
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 1)
	})
	t.Run("update host", func(t *testing.T) {
		cfg := &Cfg{
			FileHosts: []*Host{
				{URL: "test-1"},
			},
		}
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 1)
		require.Equal(t, "test-1", cfg.Hosts[0].URL)

		cfg.FileHosts[0].URL = "test-2"
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 1)
		require.Equal(t, "test-2", cfg.Hosts[0].URL)
	})
}

func TestConfig_Reload(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		jsonFile, _ := os.CreateTemp("/tmp", "*.json")
		defer func() {
			_ = os.Remove(jsonFile.Name())
		}()
		_, _ = jsonFile.Write([]byte(`{"hosts": [{"url": "test"}]}`))

		cfg := &Cfg{
			path: jsonFile.Name(),
		}
		require.NoError(t, cfg.Parse())
		require.NoError(t, cfg.Validate())

		addr := fmt.Sprintf("%p", cfg.Hosts[0])
		require.Equal(t, "test", cfg.Hosts[0].URL)

		_, _ = jsonFile.WriteAt([]byte(`{"hosts": [{"url": "test", "name": "test", "hidden": true, "description": "test", "group": "test", "method": "test"}]}`), 0)
		require.NoError(t, cfg.Parse())
		require.NoError(t, cfg.Validate())

		require.Equal(t, "test", *cfg.Hosts[0].Name)
		require.Equal(t, "test", *cfg.Hosts[0].Description)
		require.Equal(t, "test", *cfg.Hosts[0].Group)
		require.Equal(t, "test", cfg.Hosts[0].Method)
		require.True(t, cfg.Hosts[0].Hidden)

		nextAddr := fmt.Sprintf("%p", cfg.Hosts[0])
		require.Equal(t, addr, nextAddr)
	})
	t.Run("yaml", func(t *testing.T) {
		yamlFile, _ := os.CreateTemp("/tmp", "*.yaml")
		defer func() {
			_ = os.Remove(yamlFile.Name())
		}()
		_, _ = yamlFile.Write([]byte(`hosts: 
- url: test`))

		cfg := &Cfg{
			path: yamlFile.Name(),
		}
		require.NoError(t, cfg.Parse())
		require.NoError(t, cfg.Validate())

		require.Equal(t, "test", cfg.Hosts[0].URL)
		addr := fmt.Sprintf("%p", cfg.Hosts[0])

		_, _ = yamlFile.WriteAt([]byte(`hosts:
- name: test
  description: test
  group: test
  method: test
  url: test
  hidden: true`), 0)
		require.NoError(t, cfg.Parse())
		require.NoError(t, cfg.Validate())

		require.Equal(t, "test", *cfg.Hosts[0].Name)
		require.Equal(t, "test", *cfg.Hosts[0].Description)
		require.Equal(t, "test", *cfg.Hosts[0].Group)
		require.Equal(t, "test", cfg.Hosts[0].Method)
		require.True(t, cfg.Hosts[0].Hidden)

		nextAddr := fmt.Sprintf("%p", cfg.Hosts[0])
		require.NotEqual(t, addr, nextAddr)
	})

	t.Run("hosts order", func(t *testing.T) {
		file, _ := os.CreateTemp("/tmp", "*.json")
		defer os.Remove(file.Name())
		_, _ = file.Write([]byte(`{"hosts": [{"url": "test-1"}, {"url": "test-2"}]}`))

		cfg := &Cfg{path: file.Name()}
		require.NoError(t, cfg.Parse())
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 2)

		require.Equal(t, 0, cfg.Hosts[0].Index)
		require.Equal(t, 1, cfg.Hosts[1].Index)

		_, _ = file.WriteAt([]byte(`{"hosts": [{"url": "test-1"}, {"url": "test-3"}, {"url": "test-2"}]}`), 0)
		require.NoError(t, cfg.Parse())
		require.NoError(t, cfg.Validate())
		require.Len(t, cfg.Hosts, 3)

		require.Equal(t, 0, cfg.Hosts[0].Index)
		require.Equal(t, 1, cfg.Hosts[1].Index)
		require.Equal(t, 2, cfg.Hosts[2].Index)
	})
}

func TestConfig_Validate_NilHostHeaders(t *testing.T) {
	cfg := &Cfg{
		Headers: map[string]string{
			"Authorization": "Bearer token",
		},
		FileHosts: []*Host{
			{
				URL: "http://example.com",
				// Headers is nil — should not panic
			},
		},
	}

	require.NoError(t, cfg.Validate())
	require.NotNil(t, cfg.Hosts[0].Headers)
	require.Equal(t, "Bearer token", cfg.Hosts[0].Headers["Authorization"])
}

func TestConfig_Validate_DeprecatedAlerts(t *testing.T) {
	initMsg := true
	cfg := &Cfg{
		Alerts: &Notifications{
			Slack: &Slack{
				Channel: "test",
				Token:   "test-token",
			},
			InitializationMessage: &initMsg,
		},
		FileHosts: []*Host{
			{URL: "http://example.com"},
		},
	}

	require.NoError(t, cfg.Validate())
	require.NotNil(t, cfg.Notifications.Slack)
	require.Equal(t, "test", cfg.Notifications.Slack.Channel)
	require.Equal(t, "test-token", cfg.Notifications.Slack.Token)
}

func TestConfig_Validate_MaxConn(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := &Cfg{
			FileHosts: []*Host{{URL: "test"}},
		}
		require.NoError(t, cfg.Validate())
		require.Equal(t, 128, cfg.MaxConn)
	})
	t.Run("custom", func(t *testing.T) {
		cfg := &Cfg{
			MaxConn:   10,
			FileHosts: []*Host{{URL: "test"}},
		}
		require.NoError(t, cfg.Validate())
		require.Equal(t, 10, cfg.MaxConn)
	})
}

func TestConfig_Validate_HostConditionsPartialOverride(t *testing.T) {
	body := "ok"
	cfg := &Cfg{
		Conditions: &Success{
			Code: []int{200, 201},
		},
		FileHosts: []*Host{
			{
				URL: "test",
				Conditions: &Success{
					Body: &body,
					// Code is empty — should inherit from global
				},
			},
		},
	}
	require.NoError(t, cfg.Validate())
	require.Equal(t, []int{200, 201}, cfg.Hosts[0].Conditions.Code)
	require.Equal(t, &body, cfg.Hosts[0].Conditions.Body)
}

func TestConfig_Validate_HostPerHostOverrides(t *testing.T) {
	globalInterval := 30 * time.Second
	hostInterval := 10 * time.Second
	hostTimeout := 5 * time.Second

	cfg := &Cfg{
		Interval: globalInterval,
		FileHosts: []*Host{
			{
				URL:              "test",
				Interval:         &hostInterval,
				TimeoutInterval:  &hostTimeout,
				SuccessThreshold: 5,
				FailureThreshold: 10,
			},
		},
	}
	require.NoError(t, cfg.Validate())
	require.Equal(t, hostInterval, *cfg.Hosts[0].Interval)
	require.Equal(t, hostTimeout, *cfg.Hosts[0].TimeoutInterval)
	require.Equal(t, 5, cfg.Hosts[0].SuccessThreshold)
	require.Equal(t, 10, cfg.Hosts[0].FailureThreshold)
}

func TestConfig_Validate_MultipleHosts(t *testing.T) {
	cfg := &Cfg{
		FileHosts: []*Host{
			{URL: "http://a.com"},
			{URL: "http://b.com"},
			{URL: "http://c.com"},
		},
	}
	require.NoError(t, cfg.Validate())
	require.Len(t, cfg.Hosts, 3)

	for i, h := range cfg.Hosts {
		require.Equal(t, i, h.Index)
		require.NotEmpty(t, h.ID)
		require.NotNil(t, h.Interval)
		require.NotNil(t, h.TimeoutInterval)
	}
}
