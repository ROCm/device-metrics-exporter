"""Configuration file for the Sphinx documentation builder."""

external_projects_local_file = "projects.yaml"
external_projects_remote_repository = ""
external_projects = ["device-metrics-exporter"]
external_projects_current_project = "device-metrics-exporter"

project = "AMD Instinct Hub"
version = "1.5.2"
debian_version = "1.5.2"
release = version
html_title = f"AMD Device Metrics Exporter {version}"
author = "Advanced Micro Devices, Inc."
copyright = "Copyright (c) 2024 Advanced Micro Devices, Inc. All rights reserved."

# Required settings
html_theme = "rocm_docs_theme"
html_theme_options = {
    "flavor": "instinct-design",
    "use_download_button": True,
    "repository_url": "https://github.com/rocm/device-metrics-exporter",
    # Add any additional theme options here
    "announcement": (
        "You're viewing documentation from a development branch. "
        "Please switch to the release branch for the officially released documentation."
    ),
    "show_navbar_depth": 0,
}

extensions = [
    "rocm_docs",
]

# Table of contents
external_toc_path = "./sphinx/_toc.yml"

exclude_patterns = ['.venv']

# Generate llms.txt and llms-full.txt after each build (the llms.txt standard,
# https://llmstxt.org/). See the rocm-docs-core guide:
# https://rocm.docs.amd.com/projects/rocm-docs-core/en/latest/user_guide/llms.html
rocm_docs_generate_llms = True

# Supported linux version numbers
ubuntu_version_numbers = [('24.04', 'noble'), ('22.04', 'jammy')]

