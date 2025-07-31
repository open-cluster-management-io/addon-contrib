#!/usr/bin/env python3
"""
Script to update cluster-permission.yaml with latest rules from Kueue repository.
"""

import requests
import yaml
import re
import sys
from pathlib import Path

def fetch_kueue_rules():
    """Fetch the latest multikueue script from Kueue repository."""
    url = "https://raw.githubusercontent.com/kubernetes-sigs/kueue/main/site/static/examples/multikueue/create-multikueue-kubeconfig.sh"
    
    try:
        response = requests.get(url, timeout=30)
        response.raise_for_status()
        return response.text
    except requests.RequestException as e:
        print(f"Error fetching Kueue script: {e}")
        return None

def extract_rbac_rules(script_content):
    """Extract RBAC rules from the shell script."""
    # Look for the ClusterRole section in the script
    pattern = r'apiVersion: rbac\.authorization\.k8s\.io/v1\s*\nkind: ClusterRole\s*\nmetadata:\s*\n.*?\nrules:\s*\n(.*?)(?=\n---|\nEOF|\napiVersion:)'
    match = re.search(pattern, script_content, re.DOTALL)
    
    if not match:
        print("No ClusterRole rules found in the Kueue script")
        return None
    
    rules_content = match.group(1)
    
    # Create a complete YAML structure for the ClusterRole
    yaml_content = f"""apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: multikueue-role
rules:
{rules_content}"""
    
    try:
        # Parse the YAML content
        rules = yaml.safe_load(yaml_content)
        return rules
    except yaml.YAMLError as e:
        print(f"Error parsing YAML from Kueue script: {e}")
        print(f"YAML content: {yaml_content}")
        return None

def update_cluster_permission_file(file_path, new_rules):
    """Update the cluster-permission.yaml file with new rules."""
    try:
        with open(file_path, 'r') as f:
            content = yaml.safe_load(f)
    except (yaml.YAMLError, FileNotFoundError) as e:
        print(f"Error reading cluster-permission.yaml: {e}")
        return False
    
    # Update the rules section
    if 'spec' in content and 'clusterRole' in content['spec']:
        content['spec']['clusterRole']['rules'] = new_rules['rules']
    else:
        print("Invalid cluster-permission.yaml structure")
        return False
    
    # Write back to file
    try:
        with open(file_path, 'w') as f:
            yaml.dump(content, f, default_flow_style=False, sort_keys=False)
        return True
    except IOError as e:
        print(f"Error writing to cluster-permission.yaml: {e}")
        return False

def main():
    """Main function to update cluster-permission.yaml."""
    print("Fetching latest cluster-permission rules from Kueue repository...")
    
    # Fetch the Kueue script
    script_content = fetch_kueue_rules()
    if not script_content:
        sys.exit(1)
    
    # Extract RBAC rules
    print("Extracting RBAC rules from the script...")
    new_rules = extract_rbac_rules(script_content)
    if not new_rules:
        sys.exit(1)
    
    # Update the cluster-permission.yaml file
    file_path = Path("manifests/cluster-permission/cluster-permission.yaml")
    print(f"Updating {file_path} with latest rules...")
    
    if update_cluster_permission_file(file_path, new_rules):
        print("Successfully updated cluster-permission.yaml with latest rules from Kueue repository")
    else:
        print("Failed to update cluster-permission.yaml")
        sys.exit(1)

if __name__ == "__main__":
    main() 
