import json
import os

def generate_mermaid():
    blast_radius_path = "docs/index/BLAST_RADIUS.json"
    output_path = "docs/index/SYSTEM_MAP.md"
    
    if not os.path.exists(blast_radius_path):
        print(f"Error: {blast_radius_path} not found")
        return

    with open(blast_radius_path, 'r') as f:
        data = json.load(f)

    mermaid = ["# System Topology Map\n", "```mermaid", "graph TD"]
    
    # 1. Map Event Flows (Transactional Outbox)
    for topic, flow in data.get("event_flow", {}).items():
        producer = flow["producer"]
        mermaid.append(f"    {producer} -- \"{topic}\" --> Kafka")
        for consumer in flow["consumers"]:
            mermaid.append(f"    Kafka -- \"{topic}\" --> {consumer}")

    # 2. Map Shared Library Dependencies
    for pkg, info in data.get("shared_packages", {}).items():
        pkg_name = pkg.split('/')[-1]
        for consumer in info["consumers"]:
            if consumer == "all services":
                continue # Avoid cluttering the map with 'all'
            mermaid.append(f"    {consumer} -. \"uses {pkg_name}\" .-> SharedLib")

    mermaid.append("    subgraph SharedLogic\n        SharedLib\n    end")
    mermaid.append("    subgraph EventBus\n        Kafka\n    end")
    mermaid.append("```")

    with open(output_path, 'w') as f:
        f.write("\n".join(mermaid))
    
    print(f"✅ System Map generated at {output_path}")

if __name__ == "__main__":
    generate_mermaid()
