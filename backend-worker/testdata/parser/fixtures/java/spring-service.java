package com.example.users;

import java.util.List;
import java.util.Optional;
import java.util.stream.Collectors;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.stereotype.Service;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.client.RestTemplate;

/**
 * Service layer for user management with external profile enrichment.
 */
@Service
public class UserService {

    private final UserRepository repository;
    private final RestTemplate restTemplate;

    @Autowired
    public UserService(UserRepository repository, RestTemplate restTemplate) {
        this.repository = repository;
        this.restTemplate = restTemplate;
    }

    /**
     * Return all users, optionally filtered by active status.
     *
     * @param activeOnly when true, only return active users
     * @return list of user DTOs
     */
    public List<UserDTO> listUsers(boolean activeOnly) {
        List<User> users = repository.findAll();
        if (activeOnly) {
            users = users.stream()
                    .filter(User::isActive)
                    .collect(Collectors.toList());
        }
        return users.stream()
                .map(this::toDTO)
                .collect(Collectors.toList());
    }

    /**
     * Fetch a single user by ID.
     *
     * @param id the user identifier
     * @return the user DTO wrapped in Optional
     */
    public Optional<UserDTO> findById(long id) {
        return repository.findById(id).map(this::toDTO);
    }

    /**
     * Create a new user and enrich their profile from an external service.
     *
     * @param request the creation request DTO
     * @return the newly created user DTO
     */
    public UserDTO createUser(CreateUserRequest request) {
        User user = new User();
        user.setName(request.getName());
        user.setEmail(request.getEmail());
        user.setActive(true);

        // Enrich profile from external service
        String profileUrl = "https://api.profiles.example.com/v1/enrich?email=" + request.getEmail();
        ProfileData profile = restTemplate.getForObject(profileUrl, ProfileData.class);
        if (profile != null) {
            user.setAvatarUrl(profile.getAvatarUrl());
            user.setBio(profile.getBio());
        }

        User saved = repository.save(user);
        return toDTO(saved);
    }

    /**
     * Deactivate a user account.
     *
     * @param id user identifier
     * @return true if the user was found and deactivated
     */
    public boolean deactivateUser(long id) {
        Optional<User> userOpt = repository.findById(id);
        if (userOpt.isEmpty()) {
            return false;
        }
        User user = userOpt.get();
        user.setActive(false);
        repository.save(user);
        return true;
    }

    private UserDTO toDTO(User user) {
        return new UserDTO(
                user.getId(),
                user.getName(),
                user.getEmail(),
                user.isActive(),
                user.getAvatarUrl()
        );
    }
}

/**
 * REST controller exposing user endpoints.
 */
@RestController
@RequestMapping("/api/users")
class UserController {

    private final UserService userService;

    @Autowired
    UserController(UserService userService) {
        this.userService = userService;
    }

    @GetMapping
    public List<UserDTO> list(@RequestParam(defaultValue = "false") boolean activeOnly) {
        return userService.listUsers(activeOnly);
    }

    @GetMapping("/{id}")
    public ResponseEntity<UserDTO> get(@PathVariable long id) {
        return userService.findById(id)
                .map(ResponseEntity::ok)
                .orElse(ResponseEntity.notFound().build());
    }

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public UserDTO create(@RequestBody CreateUserRequest request) {
        return userService.createUser(request);
    }

    @DeleteMapping("/{id}")
    public ResponseEntity<Void> deactivate(@PathVariable long id) {
        boolean found = userService.deactivateUser(id);
        return found ? ResponseEntity.noContent().build() : ResponseEntity.notFound().build();
    }
}
