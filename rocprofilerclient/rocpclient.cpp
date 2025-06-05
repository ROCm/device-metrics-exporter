// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc. All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.


#include <unistd.h>

#include <atomic>
#include <chrono>
#include <cstdint>
#include <fstream>
#include <iostream>
#include <map>
#include <memory>
#include <sstream>
#include <stdexcept>
#include <unordered_map>
#include <vector>

#include "RocpCounterSampler.h"

template <typename Callable>
void RocprofilerCall(Callable&& callable, const std::string& msg, const char* file, int line) {
  auto result = callable();
  if (result != ROCPROFILER_STATUS_SUCCESS) {
    std::string status_msg = rocprofiler_get_status_string(result);
    ROCP_LOG(ROCP_ERROR, "[CALL][" << file << ":" << line << "] " << msg << " failed with error code "
                                 << result << ": " << status_msg << std::endl);
    std::stringstream errmsg{};
    errmsg << "[CALL][" << file << ":" << line << "] " << msg << " failure (" << status_msg << ")";
    throw std::runtime_error(errmsg.str());
  }
}

namespace amd {
namespace rocp {

std::vector<std::string> all_fields = {
		"GRBM_GUI_ACTIVE", 
		"SQ_WAVES",
		"GRBM_COUNT",
		"GPU_UTIL",
		"FETCH_SIZE",
		"WRITE_SIZE",
		"TOTAL_16_OPS",
		"TOTAL_32_OPS",
		"TOTAL_64_OPS",
		"CPC_CPC_STAT_BUSY",
		"CPC_CPC_STAT_IDLE",
		"CPC_CPC_STAT_STALL",
		"CPC_CPC_TCIU_BUSY",
		"CPC_CPC_TCIU_IDLE",
		"CPC_CPC_UTCL2IU_BUSY",
		"CPC_CPC_UTCL2IU_IDLE",
		"CPC_CPC_UTCL2IU_STALL",
		"CPC_ME1_BUSY_FOR_PACKET_DECODE",
		"CPC_ME1_DC0_SPI_BUSY",
		"CPC_UTCL1_STALL_ON_TRANSLATION",
		"CPC_ALWAYS_COUNT",
		"CPC_ADC_VALID_CHUNK_NOT_AVAIL",
		"CPC_ADC_DISPATCH_ALLOC_DONE",
		"CPC_ADC_VALID_CHUNK_END",
		"CPC_SYNC_FIFO_FULL_LEVEL",
		"CPC_SYNC_FIFO_FULL",
		"CPC_GD_BUSY",
		"CPC_TG_SEND",
		"CPC_WALK_NEXT_CHUNK",
		"CPC_STALLED_BY_SE0_SPI",
		"CPC_STALLED_BY_SE1_SPI",
		"CPC_STALLED_BY_SE2_SPI",
		"CPC_STALLED_BY_SE3_SPI",
		"CPC_LTE_ALL",
		"CPC_SYNC_WRREQ_FIFO_BUSY",
		"CPC_CANE_BUSY",
		"CPC_CANE_STALL",
		"CPF_CMP_UTCL1_STALL_ON_TRANSLATION",
		"CPF_CPF_STAT_BUSY",
		"CPF_CPF_STAT_IDLE",
		"CPF_CPF_STAT_STALL",
		"CPF_CPF_TCIU_BUSY",
		"CPF_CPF_TCIU_IDLE",
		"CPF_CPF_TCIU_STALL"
}; 
std::vector<std::shared_ptr<CounterSampler>> CounterSampler::samplers_;

std::vector<std::shared_ptr<CounterSampler>>& CounterSampler::get_samplers() { return samplers_; }

CounterSampler::CounterSampler(rocprofiler_agent_id_t agent) : agent_(agent) {
  // Setup context (should only be done once per agent)
  auto client_thread = rocprofiler_callback_thread_t{};
  RocprofilerCall([&]() { return rocprofiler_create_context(&ctx_); }, "context creation failed",
                  __FILE__, __LINE__);

  RocprofilerCall(
      [&]() {
        return rocprofiler_configure_device_counting_service(
            ctx_, {.handle = 0}, agent,
            [](rocprofiler_context_id_t context_id, rocprofiler_agent_id_t,
               rocprofiler_agent_set_profile_callback_t set_config, void* user_data) {
              if (user_data) {
                auto* sampler = static_cast<CounterSampler*>(user_data);
                sampler->set_profile(context_id, set_config);
              }
            },
            this);
      },
      "Could not setup buffered service", __FILE__, __LINE__);
}

CounterSampler::~CounterSampler() { ctx_ = {}; }

const std::string& CounterSampler::decode_record_name(
    const rocprofiler_record_counter_t& rec) const {
  static auto roc_counters = [this]() {
    auto name_to_id = CounterSampler::get_supported_counters(agent_);
    std::map<uint64_t, std::string> id_to_name;
    for (const auto& [name, id] : name_to_id) {
      id_to_name.emplace(id.handle, name);
    }
    return id_to_name;
  }();
  rocprofiler_counter_id_t counter_id = {.handle = 0};
  rocprofiler_query_record_counter_id(rec.id, &counter_id);

  auto it = roc_counters.find(counter_id.handle);
  if (it == roc_counters.end()) {
    ROCP_LOG(ROCP_ERROR, "Error: Counter handle " << counter_id.handle
                                                << " not found in roc_counters." << std::endl);
    throw std::runtime_error("Counter handle not found in roc_counters");
  }

  return it->second;
}

std::unordered_map<std::string, size_t> CounterSampler::get_record_dimensions(
    const rocprofiler_record_counter_t& rec) {
  std::unordered_map<std::string, size_t> out;
  rocprofiler_counter_id_t counter_id = {.handle = 0};
  rocprofiler_query_record_counter_id(rec.id, &counter_id);
  auto dims = get_counter_dimensions(counter_id);

  for (auto& dim : dims) {
    size_t pos = 0;
    rocprofiler_query_record_dimension_position(rec.id, dim.id, &pos);
    out.emplace(dim.name, pos);
  }
  return out;
}

void CounterSampler::sample_counter_values(const std::vector<std::string>& counters,
                                           std::vector<rocprofiler_record_counter_t>& out,
                                           uint64_t duration) {
  auto profile_cached = cached_profiles_.find(counters);
  if (profile_cached == cached_profiles_.end()) {
    size_t expected_size = 0;
    rocprofiler_profile_config_id_t profile = {};
    std::vector<rocprofiler_counter_id_t> gpu_counters;
    auto roc_counters = get_supported_counters(agent_);
    for (const auto& counter : counters) {
      auto it = roc_counters.find(counter);
      if (it == roc_counters.end()) {
        //ROCP_LOG(ROCP_ERROR, "Counter " << counter << " not found\n");
        continue;
      }
      gpu_counters.push_back(it->second);
      expected_size += get_counter_size(it->second);
    }
    RocprofilerCall(
        [&]() {
          return rocprofiler_create_profile_config(agent_, gpu_counters.data(), gpu_counters.size(),
                                                   &profile);
        },
        "Could not create profile", __FILE__, __LINE__);
    cached_profiles_.emplace(counters, profile);
    profile_sizes_.emplace(profile.handle, expected_size);
    profile_cached = cached_profiles_.find(counters);
  }

  if (profile_sizes_.find(profile_cached->second.handle) == profile_sizes_.end()) {
    ROCP_LOG(ROCP_ERROR, "Error: Profile handle " << profile_cached->second.handle
                                                << " not found in profile_sizes_." << std::endl);
    throw std::runtime_error("Profile handle not found in profile_sizes_");
  }
  out.resize(profile_sizes_.at(profile_cached->second.handle));
  profile_ = profile_cached->second;
  rocprofiler_start_context(ctx_);
  size_t out_size = out.size();
  // Wait for sampling window to collect metrics
  usleep(duration);
  rocprofiler_sample_device_counting_service(ctx_, {}, ROCPROFILER_COUNTER_FLAG_NONE, out.data(),
                                             &out_size);
  rocprofiler_stop_context(ctx_);
  out.resize(out_size);
}

std::vector<rocprofiler_agent_v0_t> CounterSampler::get_available_agents() {
  std::vector<rocprofiler_agent_v0_t> agents;
  rocprofiler_query_available_agents_cb_t iterate_cb = [](rocprofiler_agent_version_t agents_ver,
                                                          const void** agents_arr,
                                                          size_t num_agents, void* udata) {
    if (agents_ver != ROCPROFILER_AGENT_INFO_VERSION_0)
      throw std::runtime_error{"unexpected rocprofiler agent version"};
    auto* agents_v = static_cast<std::vector<rocprofiler_agent_v0_t>*>(udata);
    for (size_t i = 0; i < num_agents; ++i) {
      const auto* rocp_agent = static_cast<const rocprofiler_agent_v0_t*>(agents_arr[i]);
      if (rocp_agent->type == ROCPROFILER_AGENT_TYPE_GPU) agents_v->emplace_back(*rocp_agent);
    }
    return ROCPROFILER_STATUS_SUCCESS;
  };

  RocprofilerCall(
      [&]() {
        return rocprofiler_query_available_agents(
            ROCPROFILER_AGENT_INFO_VERSION_0, iterate_cb, sizeof(rocprofiler_agent_t),
            const_cast<void*>(static_cast<const void*>(&agents)));
      },
      "query available agents", __FILE__, __LINE__);
  return agents;
}

int CounterSampler::runSample(std::vector<std::string> &metric_fields) 
{
	std::vector<std::shared_ptr<CounterSampler>> samplers = {};
	std::vector<rocprofiler_agent_v0_t> agents;
	// populate list of agents
	agents = CounterSampler::get_available_agents();
	samplers = CounterSampler::get_samplers();
	std::vector<std::string> metrics;
	if (metric_fields.size() == 0) {
		metrics = all_fields;
	} else {
		metrics = metric_fields;
	}

	// find intersection of supported and requested fields
	std::cout << "{\n\"GpuMetrics\": \[\n\t";
	for (uint32_t gpu_index = 0; gpu_index < agents.size(); gpu_index++) {
		auto& cs = *samplers[gpu_index];
		std::cout << "{\"GpuId\" : " << "\"" << gpu_index << "\",\n";
		std::cout << "\t\"Metrics\" : \[\n";
		uint32_t count = 0;
		for (uint32_t mindex = 0; mindex < metrics.size(); mindex++) {
			thread_local std::vector<rocprofiler_record_counter_t> records;
			try {
				cs.sample_counter_values({metrics[mindex]} , records, 10);
			} catch (const std::exception& e) {
				ROCP_LOG(ROCP_ERROR, "Error while sampling counter values: " << e.what());
				return -1;
			}
			if (records.size() == 0) {
				// skip unsupported fields
				continue;
			}
			count++;
			if ( count != 1) {
				std::cout <<",\n";
			}
			// Aggregate counter values. Rocprof v1/v2 summed values across dimensions.
			double value = 0.0;
			for (auto& record : records) {
				value += record.counter_value;  // Summing up values from all dimensions.
			}
			std::cout << "\t\t\{\n";
			std::cout << "\t\t\t\"Field\" : \"" << metrics[mindex] << "\", \"Value\": \"" << value << "\"\n";
			std::cout << "\t\t}";
			//std::clog << metrics[mindex] << " value : " << value << "\n";
			//gpu->list[mindex].value = value;
		}
		std::cout << "\n\t]}\n";
		if (gpu_index+1 != agents.size()) {
			std::cout << ",";
		}
	}
	std::cout << "]\n}\n";
	return 0;
}

void CounterSampler::set_profile(rocprofiler_context_id_t ctx,
                                 rocprofiler_agent_set_profile_callback_t cb) const {
  if (profile_.handle != 0) {
    cb(ctx, profile_);
  }
}

size_t CounterSampler::get_counter_size(rocprofiler_counter_id_t counter) {
  size_t size = 1;
  rocprofiler_iterate_counter_dimensions(
      counter,
      [](rocprofiler_counter_id_t, const rocprofiler_record_dimension_info_t* dim_info,
         size_t num_dims, void* user_data) {
        size_t* s = static_cast<size_t*>(user_data);
        for (size_t i = 0; i < num_dims; i++) {
          *s *= dim_info[i].instance_size;
        }
        return ROCPROFILER_STATUS_SUCCESS;
      },
      static_cast<void*>(&size));
  return size;
}

std::unordered_map<std::string, rocprofiler_counter_id_t> CounterSampler::get_supported_counters(
    rocprofiler_agent_id_t agent) {
  std::unordered_map<std::string, rocprofiler_counter_id_t> out;
  std::vector<rocprofiler_counter_id_t> gpu_counters;

  RocprofilerCall(
      [&]() {
        return rocprofiler_iterate_agent_supported_counters(
            agent,
            [](rocprofiler_agent_id_t, rocprofiler_counter_id_t* counters, size_t num_counters,
               void* user_data) {
              std::vector<rocprofiler_counter_id_t>* vec =
                  static_cast<std::vector<rocprofiler_counter_id_t>*>(user_data);
              for (size_t i = 0; i < num_counters; i++) {
                vec->push_back(counters[i]);
              }
              return ROCPROFILER_STATUS_SUCCESS;
            },
            static_cast<void*>(&gpu_counters));
      },
      "Could not fetch supported counters", __FILE__, __LINE__);
  for (auto& counter : gpu_counters) {
    rocprofiler_counter_info_v0_t version;
    RocprofilerCall(
        [&]() {
          return rocprofiler_query_counter_info(counter, ROCPROFILER_COUNTER_INFO_VERSION_0,
                                                static_cast<void*>(&version));
        },
        "Could not query info for counter", __FILE__, __LINE__);
    out.emplace(version.name, counter);
  }
  return out;
}

std::vector<rocprofiler_record_dimension_info_t> CounterSampler::get_counter_dimensions(
    rocprofiler_counter_id_t counter) {
  std::vector<rocprofiler_record_dimension_info_t> dims;
  rocprofiler_available_dimensions_cb_t cb = [](rocprofiler_counter_id_t,
                                                const rocprofiler_record_dimension_info_t* dim_info,
                                                size_t num_dims, void* user_data) {
    std::vector<rocprofiler_record_dimension_info_t>* vec =
        static_cast<std::vector<rocprofiler_record_dimension_info_t>*>(user_data);
    for (size_t i = 0; i < num_dims; i++) {
      vec->push_back(dim_info[i]);
    }
    return ROCPROFILER_STATUS_SUCCESS;
  };
  RocprofilerCall([&]() { return rocprofiler_iterate_counter_dimensions(counter, cb, &dims); },
                  "Could not iterate counter dimensions", __FILE__, __LINE__);
  return dims;
}

int tool_init(rocprofiler_client_finalize_t, void*) {
  // Get the agents available on the device
  auto agents = CounterSampler::get_available_agents();
  if (agents.empty()) {
    ROCP_LOG(ROCP_ERROR, "No agents found\n");
    return -1;
  }

  for (auto agent : agents) {
    CounterSampler::get_samplers().push_back(std::make_shared<CounterSampler>(agent.id));
  }

  // no errors
  return 0;
}

void tool_fini(void* user_data) {
  auto* output_stream = static_cast<std::ostream*>(user_data);
  *output_stream << std::flush;
  if (output_stream != &std::cout && output_stream != &std::cerr) delete output_stream;
}

extern "C" rocprofiler_tool_configure_result_t* rocprofiler_configure(uint32_t version,
                                                                      const char* runtime_version,
                                                                      uint32_t priority,
                                                                      rocprofiler_client_id_t* id) {
  // set the client name
  id->name = "rocpclient";

  // compute major/minor/patch version info
  uint32_t major = version / 10000;
  uint32_t minor = (version % 10000) / 100;
  uint32_t patch = version % 100;

  // generate info string
  auto info = std::stringstream{};
  info << id->name << " (priority=" << priority << ") is using rocprofiler-sdk v" << major << "."
       << minor << "." << patch << " (" << runtime_version << ")";

  //std::clog << info.str() << std::endl;

  std::ostream* output_stream = nullptr;
    output_stream = &std::cout;
  static auto cfg =
      rocprofiler_tool_configure_result_t{sizeof(rocprofiler_tool_configure_result_t), &tool_init,
                                          &tool_fini, static_cast<void*>(output_stream)};

  // return pointer to configure data
  return &cfg;
}
}  // namespace rocp
}  // namespace amd
