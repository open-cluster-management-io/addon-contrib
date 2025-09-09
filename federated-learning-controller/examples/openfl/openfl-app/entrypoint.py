import argparse
import yaml
from pathlib import Path
import os
import subprocess

PLAN_FILE = Path("plan/plan.yaml")
DATA_FILE = Path("plan/data.yaml")
COLS_FILE = Path("plan/cols.yaml")


def load_plan():
    with open(PLAN_FILE, "r") as f:
        return yaml.safe_load(f)


def save_plan(config):
    with open(PLAN_FILE, "w") as f:
        yaml.dump(config, f, sort_keys=False)


def update_server(args):
    cfg = load_plan()

    if args.cols:
        if COLS_FILE.exists():
            with open(COLS_FILE, "r") as f:
                cols_config = yaml.safe_load(f) or {}
        else:
            cols_config = {}
        
        cols_list = [col.strip() for col in args.cols.split(',')]
        cols_config['collaborators'] = cols_list

        with open(COLS_FILE, "w") as f:
            yaml.dump(cols_config, f, sort_keys=False)
        print(f"[OK] Updated collaborator names in {COLS_FILE}")
        cfg["data_loader"]["settings"]["collaborator_count"] = len(cols_list)

    cfg["network"]["settings"]["use_tls"] = False

    if args.server_ip:
        cfg["network"]["settings"]["agg_addr"] = args.server_ip
    
    if args.server_port:
        cfg["network"]["settings"]["agg_port"] = args.server_port

    if args.num_rounds:
        cfg["aggregator"]["settings"]["rounds_to_train"] = int(args.num_rounds)

    if args.model_dir:
        best_file = os.path.basename(cfg["aggregator"]["settings"]["best_state_path"])
        last_file = os.path.basename(cfg["aggregator"]["settings"]["last_state_path"])
        cfg["aggregator"]["settings"]["best_state_path"] = os.path.join(args.model_dir, best_file)
        cfg["aggregator"]["settings"]["last_state_path"] = os.path.join(args.model_dir, last_file)

    save_plan(cfg)
    print(f"[OK] Updated server settings in {PLAN_FILE}")

    os.execvp("fx", ["fx", "aggregator", "start"])


def update_client(args):
    with open(DATA_FILE, "a") as f:
        f.write(f"{args.name},{args.data_path}\n")

    cfg = load_plan()
    cfg["network"]["settings"]["use_tls"] = False

    if args.server_ip:
        cfg["network"]["settings"]["agg_addr"] = args.server_ip

    if args.server_port:
        cfg["network"]["settings"]["agg_port"] = args.server_port

    if args.num_rounds:
        cfg["aggregator"]["settings"]["rounds_to_train"] = int(args.num_rounds)

    if args.model_dir:
        best_file = os.path.basename(cfg["aggregator"]["settings"]["best_state_path"])
        last_file = os.path.basename(cfg["aggregator"]["settings"]["last_state_path"])
        cfg["aggregator"]["settings"]["best_state_path"] = os.path.join(args.model_dir, best_file)
        cfg["aggregator"]["settings"]["last_state_path"] = os.path.join(args.model_dir, last_file)

    save_plan(cfg)
    print(f"[OK] Updated client settings in {PLAN_FILE}")

    os.execvp("fx", ["fx", "collaborator", "start", "-n", args.name])


def main():
    parser = argparse.ArgumentParser(description="Update OpenFL plan.yaml")

    subparsers = parser.add_subparsers(dest="role", required=True)

    # docker run --rm image server --server-port 8080 --num-rounds 3 --cols client1,client2 --model-dir models
    sp_server = subparsers.add_parser("server", help="Update server (aggregator) settings")
    sp_server.add_argument("--server-ip", help="Aggregator IP address")
    sp_server.add_argument("--server-port", type=int, help="Aggregator port")
    sp_server.add_argument("--num-rounds", type=int, help="Number of rounds to train")
    sp_server.add_argument("--cols", help="Comma-separated list of collaborator names for cols.yaml")
    sp_server.add_argument("--model-dir", help="Directory for model files")
    sp_server.set_defaults(func=update_server)

    # docker run --rm image client --name client1 --data-path /data/client1 --server-ip 172.17.0.2 --server-port 8080 --num-rounds 3 --model-dir models
    sp_client = subparsers.add_parser("client", help="Update client (collaborator) settings")
    sp_client.add_argument("--name", required=True, help="Collaborator name")
    sp_client.add_argument("--data-path", required=True, help="Path to collaborator data")
    sp_client.add_argument("--server-ip", help="Aggregator IP address")
    sp_client.add_argument("--server-port", type=int, help="Aggregator port")
    sp_client.add_argument("--num-rounds", type=int, help="Number of rounds to train")
    sp_client.add_argument("--model-dir", help="Directory for model files")
    sp_client.set_defaults(func=update_client)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()