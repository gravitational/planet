package main

import (
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
)

func TestDefaultCgroupConfig(t *testing.T) {
	tests := []struct {
		numCPU   int
		isMaster bool
		expected *CgroupConfig
	}{
		{
			numCPU:   1,
			isMaster: false,
			expected: &CgroupConfig{
				Enabled: true,
				Auto:    true,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(50000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
		{
			numCPU:   1,
			isMaster: true,
			expected: &CgroupConfig{
				Enabled: true,
				Auto:    true,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(50000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
		{
			numCPU:   6,
			isMaster: false,
			expected: &CgroupConfig{
				Enabled:                   true,
				Auto:                      true,
				KubeReservedCPUMillicores: 800,
				KubeSystemCPUMillicores:   800,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(60000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
		{
			numCPU:   6,
			isMaster: true,
			expected: &CgroupConfig{
				Enabled:                   true,
				Auto:                      true,
				KubeReservedCPUMillicores: 1800,
				KubeSystemCPUMillicores:   800,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(60000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
		{
			numCPU:   10,
			isMaster: false,
			expected: &CgroupConfig{
				Enabled:                   true,
				Auto:                      true,
				KubeReservedCPUMillicores: 800,
				KubeSystemCPUMillicores:   1200,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(100000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
		{
			numCPU:   10,
			isMaster: true,
			expected: &CgroupConfig{
				Enabled:                   true,
				Auto:                      true,
				KubeReservedCPUMillicores: 1800,
				KubeSystemCPUMillicores:   1200,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(100000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
		{
			numCPU:   20,
			isMaster: false,
			expected: &CgroupConfig{
				Enabled:                   true,
				Auto:                      true,
				KubeReservedCPUMillicores: 800,
				KubeSystemCPUMillicores:   2200,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(200000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
		{
			numCPU:   20,
			isMaster: true,
			expected: &CgroupConfig{
				Enabled:                   true,
				Auto:                      true,
				KubeReservedCPUMillicores: 1800,
				KubeSystemCPUMillicores:   2200,
				Cgroups: []CgroupEntry{
					CgroupEntry{
						Path: "user",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
								Quota:  i64(200000),
								Period: u64(DefaultCgroupCPUPeriod),
							},
						},
					},
					CgroupEntry{
						Path: "system.slice",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(1024),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(0),
							},
						},
					},
					CgroupEntry{
						Path: "kube-pods",
						LinuxResources: specs.LinuxResources{
							CPU: &specs.LinuxCPU{
								Shares: u64(2),
							},
							Memory: &specs.LinuxMemory{
								Swappiness: u64(20),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		config := defaultCgroupConfig(tt.numCPU, tt.isMaster)
		assert.Equal(t, tt.expected, config, "cpu: %v is_master: %v", tt.numCPU, tt.isMaster)
	}
}
