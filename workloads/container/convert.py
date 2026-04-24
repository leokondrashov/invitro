import json
import re
from pathlib import Path

base = Path('workloads/container/yamls')
deploy_info_path = base / 'deploy_info.json'

def parse_relay_args(lines):
    args = {}
    for line in lines:
        m = re.search(r'-\s+--([^=\s]+)=(.*)\s*$', line)
        if m:
            k, v = m.group(1), m.group(2).strip()
            if (v.startswith('"') and v.endswith('"')) or (v.startswith("'") and v.endswith("'")):
                v = v[1:-1]
            args[k] = v
    return args

def remove_relay_container_and_add_port(path: Path):
    text = path.read_text()
    lines = text.splitlines()

    relay_start = None
    relay_end = None
    relay_item_indent = None
    function_item_start = None
    relay_args_lines = []

    for i, line in enumerate(lines):
        if relay_start is None:
            if re.match(r'^\s*-\s+name:\s*relay\s*$', line) or re.match(r'^\s*-\s+image:\s+.*relay:.*$', line):
                relay_start = i
                relay_item_indent = len(line) - len(line.lstrip(' '))
                continue
        else:
            # capture relay arg lines while inside relay item
            relay_args_lines.append(line)
            # first subsequent container item at same indent is end boundary
            if re.match(r'^\s*-\s+(name|image):\s*', line):
                indent = len(line) - len(line.lstrip(' '))
                if indent == relay_item_indent and i > relay_start:
                    relay_end = i
                    function_item_start = i
                    break

    if relay_start is None:
        return None, False

    if relay_end is None:
        relay_end = len(lines)

    relay_block = lines[relay_start:relay_end]
    relay_args = parse_relay_args(relay_block)

    new_lines = lines[:relay_start] + lines[relay_end:]

    # remove extra blank lines created around container splice
    while relay_start < len(new_lines)-1 and new_lines[relay_start] == '' and new_lines[relay_start+1] == '':
        del new_lines[relay_start]

    # identify first remaining container item and ensure it exposes h2c port
    container_idx = None
    item_indent = None
    for i, line in enumerate(new_lines):
        if re.match(r'^\s*-\s+(name|image):\s*', line):
            container_idx = i
            item_indent = len(line) - len(line.lstrip(' '))
            break

    if container_idx is None:
        path.write_text('\n'.join(new_lines) + '\n')
        return relay_args, True

    # Determine end of first container item
    container_end = len(new_lines)
    for j in range(container_idx + 1, len(new_lines)):
        line = new_lines[j]
        if re.match(r'^\s*-\s+(name|image):\s*', line) and (len(line) - len(line.lstrip(' '))) == item_indent:
            container_end = j
            break

    has_ports = any(re.match(r'^\s*ports:\s*$', new_lines[j]) for j in range(container_idx + 1, container_end))
    if not has_ports and 'function-endpoint-port' in relay_args:
        # insert after image line in first container if possible
        image_line_idx = None
        for j in range(container_idx, container_end):
            if re.match(r'^\s*-\s+image:\s*', new_lines[j]) or re.match(r'^\s*image:\s*', new_lines[j]):
                image_line_idx = j
                break
        if image_line_idx is not None:
            indent_item = ' ' * item_indent
            indent_field = ' ' * (item_indent + 2)
            indent_list = ' ' * (item_indent + 4)
            indent_sub = ' ' * (item_indent + 6)
            port = relay_args['function-endpoint-port']
            port_block = [
                f"{indent_field}ports:",
                f"{indent_list}- name: h2c",
                f"{indent_sub}containerPort: {port}",
            ]
            new_lines[image_line_idx+1:image_line_idx+1] = port_block

    path.write_text('\n'.join(new_lines) + '\n')
    return relay_args, True

# Process YAMLs
yaml_files = sorted(base.rglob('*.yaml'))
relay_by_yaml = {}
changed = 0
for y in yaml_files:
    if y.name == 'deploy_info.json':
        continue
    relay_args, did = remove_relay_container_and_add_port(y)
    if did:
        changed += 1
    if relay_args:
        relay_by_yaml[str(y).replace('\\','/')] = relay_args

# Update deploy_info with InvocationParams from relay args
with deploy_info_path.open() as f:
    deploy_info = json.load(f)

print(deploy_info)

for fn_name, info in deploy_info.items():
    yaml_loc = info.get('YamlLocation', '')
    yaml_abs = yaml_loc.replace('\\','/')
    print(yaml_abs)
    args = relay_by_yaml.get(yaml_abs)
    if not args:
        continue

    params = {
        'FunctionName': args.get('function-name', fn_name),
        'Generator': args.get('generator', 'unique'),
        'Value': args.get('value', ''),
        'Method': args.get('function-method', ''),
        'LowerBound': int(args.get('lowerBound', '1')),
        'UpperBound': int(args.get('upperBound', args.get('lowerBound', '10'))),
    }
    if 'function-endpoint-port' in args:
        params['EndpointPort'] = int(args['function-endpoint-port'])
    info['InvocationParams'] = params
    print(info)

with deploy_info_path.open('w') as f:
    json.dump(deploy_info, f, indent=4)
    f.write('\n')

print(f'processed_yaml={len(yaml_files)} changed_yaml={changed} with_relay_args={len(relay_by_yaml)}')