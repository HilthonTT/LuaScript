local function add(a, b)
    assert(type(a) == "number", "a must be a number")
    assert(type(b) == "number", "a must be a number")

    return a + b
end

local Entity = {}
Entity.__index = Entity

function Entity.new(id, name, description, children)
    return setmetatable({
        id = id,
        name = name,
        description = description,
        children = children,
    })
end

local function main()
    local a = 20
    local b = 35

    print(add(a, b))

    local e_1 = Entity.new(1, "First Entity", "Very descriptif", {})
    local e_2 = Entity.new(1, "Second Entity", "Not descriptif", { e_1 = e_1 })

    print(e_2.name)
    print(e_2.description)
    print(e_2.children.e_1.name)
end

main()
